package admin

import (
	"errors"
	"fmt"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/policy"
	"gorm.io/gorm"
)

// EnsureEndpointPermissions inserts the API and button permission rows
// declared by generated business services. It is idempotent: rows are
// matched by (type, data) and existing rows are never overwritten, so
// operator edits to description/group survive a restart.
//
// Call it after Server.Setup (which creates sys_permissions) and before
// the HTTP server starts. A typical bootstrap calls:
//
//	admin.EnsureEndpointPermissions(db, pb.OrderServicePolicies...)
//
// Public policies are skipped: they are meant to be added to the JWT
// and PermissionChecker allowlists by the application bootstrap.
func EnsureEndpointPermissions(db *gorm.DB, policies ...policy.Policy) error {
	if db == nil {
		return errors.New("admin: EnsureEndpointPermissions requires a non-nil *gorm.DB")
	}
	for _, p := range policies {
		if p.Public {
			continue
		}
		if data := p.Permission(); data != "" {
			if err := ensurePermission(db, string(models.PermissionTypeAPI), data, p.Description, p.Group); err != nil {
				return err
			}
		}
		if p.Button != "" {
			if err := ensurePermission(db, string(models.PermissionTypeButton), p.Button, p.Description, p.Group); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensurePermission(db *gorm.DB, typ, data, description, group string) error {
	if data == "" {
		return nil
	}
	var existing models.Permission
	err := db.Unscoped().Where("type = ? AND data = ?", typ, data).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup sys_permission %q/%q: %w", typ, data, err)
	}
	row := models.Permission{
		Type:        typ,
		Data:        data,
		Description: description,
		Group:       group,
	}
	if err := db.Create(&row).Error; err != nil {
		return fmt.Errorf("auto-create endpoint permission %q/%q: %w", typ, data, err)
	}
	return nil
}
