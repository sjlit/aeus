package admin

import (
	"context"
	"strings"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/rest/v3"
	"gorm.io/gorm"
)

// installSecurityGuards adds target-level checks that the generic REST
// layer cannot express through scenarios:
//
//   - a user update/delete whose current role is a super role requires a
//     super caller;
//   - changing any user's role requires a super caller (otherwise a user
//     manager could assign a more privileged role);
//   - a role update/delete targeting an IsSuper role requires a super
//     caller (otherwise a role manager can disable or delete the super
//     role through generic CRUD);
//   - UID and Username share one login namespace, so a user may not take
//     the other column's value (otherwise Login sees two candidates and
//     rejects both). Create checks run in the REST BeforeCreate hook;
//     update checks run here.
//
// The update/delete guards are GORM callbacks because rest/v3's local
// BeforeUpdate hook receives the request model without the primary key;
// the GORM statement still carries the previous model and the update map,
// which is what these checks need.
//
// Operations without a rest/v3 RuntimeScope (Seed, direct GORM writes,
// background jobs) are intentionally skipped. REST calls always carry the
// scope, so this keeps bootstrapping and system tooling unrestricted while
// protecting the HTTP surface.
func installSecurityGuards(db *gorm.DB) {
	if db == nil {
		return
	}

	db.Callback().Update().Before("gorm:update").Register("aeus:security:update", func(tx *gorm.DB) {
		if !guardApplies(tx) {
			return
		}
		if err := guardUpdate(tx); err != nil {
			_ = tx.AddError(err)
		}
	})

	db.Callback().Delete().Before("gorm:delete").Register("aeus:security:delete", func(tx *gorm.DB) {
		if !guardApplies(tx) {
			return
		}
		if err := guardDelete(tx); err != nil {
			_ = tx.AddError(err)
		}
	})
}

func guardApplies(tx *gorm.DB) bool {
	return tx != nil &&
		tx.Error == nil &&
		tx.Statement != nil &&
		rest.RuntimeScopeFromContext(tx.Statement.Context) != nil
}

func guardUpdate(tx *gorm.DB) error {
	switch target := statementModel(tx).(type) {
	case *models.User:
		return guardUserUpdate(tx, target)
	case *models.Role:
		if target.IsSuper && !callerIsSuper(tx.Statement.Context, tx) {
			return errs.ErrPermissionDenied
		}
	}
	return nil
}

func guardUserUpdate(tx *gorm.DB, target *models.User) error {
	ctx := tx.Statement.Context
	super := callerIsSuper(ctx, tx)

	// Any update to a super user (disable, rename, demote, ...) is a
	// super-only operation. Checking before the assignment below also
	// prevents a non-super caller from demoting a super user.
	if roleIsSuper(ctx, tx, target.RoleKey) && !super {
		return errs.ErrPermissionDenied
	}

	// UID and Username share one login namespace; enforce it on every
	// edit (a username change can collide with another row's UID just
	// as a create can).
	uid, username := updatedLoginIdentity(tx, target)
	if err := ensureLoginIdentityUnique(ctx, tx, uid, username, target.ID); err != nil {
		return err
	}

	nextRole := updatedRoleKey(tx)
	if nextRole == "" || nextRole == target.RoleKey {
		return nil
	}
	// Role changes are super-only: a user manager must not be able to
	// promote themselves or anyone else by picking a more privileged
	// role, even when that role is not IsSuper.
	if !super {
		return errs.ErrPermissionDenied
	}
	// Still validate the target role and preserve the create-path
	// "only a super caller may assign a super role" invariant (we are
	// super here, but keeping one helper avoids divergence).
	return ensureCallerMayAssignRole(ctx, tx, nextRole)
}

func guardDelete(tx *gorm.DB) error {
	switch target := deleteTarget(tx).(type) {
	case *models.User:
		if roleIsSuper(tx.Statement.Context, tx, target.RoleKey) && !callerIsSuper(tx.Statement.Context, tx) {
			return errs.ErrPermissionDenied
		}
	case *models.Role:
		if target.IsSuper && !callerIsSuper(tx.Statement.Context, tx) {
			return errs.ErrPermissionDenied
		}
	}
	return nil
}

// deleteTarget prefers Dest: GORM's delete callback may leave
// Statement.Model pointing at a zero value and put the loaded row in
// Dest instead.
func deleteTarget(tx *gorm.DB) any {
	if tx.Statement.Dest != nil {
		return tx.Statement.Dest
	}
	return tx.Statement.Model
}

// statementModel returns the loaded/declared model for an update
// statement. The previous row is carried in Statement.Model.
func statementModel(tx *gorm.DB) any {
	if tx.Statement.Model != nil {
		return tx.Statement.Model
	}
	return tx.Statement.Dest
}

// updatedLoginIdentity merges the previous row with the changed values,
// so a partial update that only touches email/status still validates the
// existing UID/username pair.
func updatedLoginIdentity(tx *gorm.DB, previous *models.User) (uid, username string) {
	uid, username = previous.UID, previous.Username
	switch dest := tx.Statement.Dest.(type) {
	case map[string]any:
		if v, ok := dest["uid"].(string); ok {
			uid = v
		}
		if v, ok := dest["username"].(string); ok {
			username = v
		}
	case *models.User:
		if dest.UID != "" {
			uid = dest.UID
		}
		if dest.Username != "" {
			username = dest.Username
		}
	}
	return uid, username
}

// ensureLoginIdentityUnique rejects a UID/username that collides with
// either column of another row. The two columns are one login namespace;
// allowing cross-column collisions makes Login's ambiguity guard a
// denial-of-service vector. excludeID skips the row being updated.
func ensureLoginIdentityUnique(ctx context.Context, db *gorm.DB, uid, username string, excludeID uint) error {
	// Login accepts either identifier, so each new value must be unique
	// in BOTH columns: a username equal to another row's uid is just as
	// ambiguous as a duplicate username. The check is deliberately global
	// (not tenant-scoped): Login has no tenant selector, so the same
	// identifier in two tenants would still make login ambiguous.
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	for _, value := range []string{uid, username} {
		if value == "" {
			continue
		}
		conditions = append(conditions, "uid = ?", "username = ?")
		args = append(args, value, value)
	}
	if len(conditions) == 0 {
		return nil
	}
	query := freshDB(WithoutTenant(ctx), db).Model(&models.User{}).
		Where("("+strings.Join(conditions, " OR ")+")", args...)
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var matches []models.User
	if err := query.Limit(1).Find(&matches).Error; err != nil {
		return err
	}
	if len(matches) > 0 {
		return errs.Newf(errs.CodeInvalid, "uid or username is already in use")
	}
	return nil
}

// updatedRoleKey extracts the new role_key from the update map/struct.
// rest/v3 builds a map of changed columns, so this only returns a value
// when the caller actually changes the role.
func updatedRoleKey(tx *gorm.DB) string {
	switch dest := tx.Statement.Dest.(type) {
	case map[string]any:
		if v, ok := dest["role_key"].(string); ok {
			return v
		}
	case *models.User:
		return dest.RoleKey
	}
	return ""
}

func callerIsSuper(ctx context.Context, db *gorm.DB) bool {
	scope := rest.RuntimeScopeFromContext(ctx)
	if scope == nil || scope.User == "" {
		return false
	}
	var caller models.User
	if err := freshDB(ctx, db).Where("uid = ?", scope.User).First(&caller).Error; err != nil {
		return false
	}
	var role models.Role
	if err := freshDB(ctx, db).Where("`key` = ?", caller.RoleKey).First(&role).Error; err != nil {
		return false
	}
	return role.IsSuper
}

func roleIsSuper(ctx context.Context, db *gorm.DB, roleKey string) bool {
	if roleKey == "" {
		return false
	}
	var role models.Role
	if err := freshDB(ctx, db).Where("`key` = ?", roleKey).First(&role).Error; err != nil {
		return false
	}
	return role.IsSuper
}

// freshDB detaches a lookup from the caller's statement (which may still
// carry the User/Role schema) while keeping the transaction connection and
// request context.
func freshDB(ctx context.Context, db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{NewDB: true, Initialized: true}).WithContext(ctx)
}
