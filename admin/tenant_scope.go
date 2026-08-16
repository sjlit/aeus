package admin

import (
	"reflect"
	"sync"

	"github.com/sjlit/aeus/admin/middleware"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// __tenantSchemaCache memoizes the tenant-column lookup against
// *schema.Schema pointers. GORM interns a single *schema.Schema per
// (db, model-type) for the lifetime of the process, so the cache
// holds at most one entry per registered tenant-aware model and
// the hot path becomes a single Load.
var __tenantSchemaCache sync.Map // map[*schema.Schema]bool

// installTenantScope wires GORM callbacks so every read/update/delete on
// a model that carries the `tenant_id` column is automatically scoped
// to the tenant id produced by the resolver, and every create on such
// a model has its `TenantID` field backfilled from the resolver when
// the caller left it blank.
//
// The callbacks run BEFORE the SQL is generated (Before("gorm:query"),
// Before("gorm:update"), Before("gorm:delete")) so the WHERE clause is
// merged into the same compiled statement. For Create, we run
// Before("gorm:create") which fires after the per-model BeforeCreate
// hooks, so explicit TenantID values set by application code win over
// the resolver-supplied default.
//
// The resolver is invoked against the *operation's* ctx on every DB
// call — there's no separate "store tenant on ctx" middleware to
// install. This means the default wiring (admin.New + middleware/auth.JWT
// + admin.Server.Setup) yields end-to-end isolation with no extra
// boilerplate: JWT middleware writes claims on ctx, the callback's
// resolver reads TenantID off the claims.
//
// A model without a `tenant_id` column (e.g. Menu, which embeds the
// plain BaseModel) is left alone: the callback bails out early. A
// resolver that returns "" also causes the callback to bail out, so
// AuthService.Login — which runs without JWT claims — can still find
// users by username/uid across tenants.
func installTenantScope(db *gorm.DB, resolver middleware.Resolver) {
	if resolver == nil {
		return
	}

	query := db.Callback().Query()
	query.Before("gorm:query").Register("aeus:tenant:query", func(tx *gorm.DB) {
		applyTenantScope(tx, resolver)
	})

	update := db.Callback().Update()
	update.Before("gorm:update").Register("aeus:tenant:update", func(tx *gorm.DB) {
		applyTenantScope(tx, resolver)
	})

	delete := db.Callback().Delete()
	delete.Before("gorm:delete").Register("aeus:tenant:delete", func(tx *gorm.DB) {
		applyTenantScope(tx, resolver)
	})

	create := db.Callback().Create()
	create.Before("gorm:create").Register("aeus:tenant:create", func(tx *gorm.DB) {
		if !hasTenantIDColumn(tx.Statement.Schema) {
			return
		}
		tid := resolver(tx.Statement.Context)
		if tid == "" {
			return
		}
		backfillTenantID(tx.Statement.ReflectValue, tid)
	})
}

// applyTenantScope is shared by Query / Update / Delete: it calls
// the resolver against the operation's ctx and, if a non-empty
// tenant id is produced, appends it as a WHERE clause.
//
// Models without a `tenant_id` column are skipped, and so are
// operations whose resolver returns "" (the explicit opt-out for
// super-admin tooling and the AuthService.Login RPC).
func applyTenantScope(tx *gorm.DB, resolver middleware.Resolver) {
	if !hasTenantIDColumn(tx.Statement.Schema) {
		return
	}
	tid := resolver(tx.Statement.Context)
	if tid == "" {
		return
	}
	tx.Statement.Where("tenant_id = ?", tid)
}

// hasTenantIDColumn returns true when the destination schema declares
// a column named `tenant_id`. We key off the column rather than the
// struct embedding `TenantModel` so that callers can drop in their own
// tenant-aware struct without depending on admin's model package.
// Result is memoized per *schema.Schema because GORM reuses the
// same pointer across statements for a given model type.
func hasTenantIDColumn(s *schema.Schema) bool {
	if s == nil {
		return false
	}
	if v, ok := __tenantSchemaCache.Load(s); ok {
		return v.(bool)
	}
	for _, f := range s.Fields {
		if f.DBName == "tenant_id" {
			__tenantSchemaCache.Store(s, true)
			return true
		}
	}
	__tenantSchemaCache.Store(s, false)
	return false
}

// modelHasTenantIDColumn reports whether model's GORM schema declares a
// tenant_id column, parsing on a fresh session so the shared handle's
// statement is never clobbered (the same hazard registerModel guards
// against).  registerModel uses it to decide whether rest/v3's
// TenantResolve may be installed: rest/v3 appends `tenant_id = ?` to
// Search/Export unconditionally, which would break on models without
// the column (Menu, Permission, Tenant).
func modelHasTenantIDColumn(db *gorm.DB, model any) bool {
	stmt := &gorm.Statement{DB: db.Session(&gorm.Session{NewDB: true})}
	if err := stmt.Parse(model); err != nil {
		return false
	}
	return hasTenantIDColumn(stmt.Schema)
}

// backfillTenantID walks rv (which may be a struct, a pointer to
// struct, or a slice of either) and sets every embedded TenantID
// field that is still the zero value. Already-set values are left
// alone so application code can override the resolver on demand.
func backfillTenantID(rv reflect.Value, tid string) {
	if !rv.IsValid() {
		return
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			backfillTenantID(rv.Index(i), tid)
		}
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return
		}
		backfillTenantID(rv.Elem(), tid)
	case reflect.Struct:
		t := rv.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				backfillTenantID(rv.Field(i), tid)
				continue
			}
			if f.Name == "TenantID" && f.Type.Kind() == reflect.String {
				fv := rv.Field(i)
				if fv.CanSet() && fv.String() == "" {
					fv.SetString(tid)
				}
			}
		}
	}
}
