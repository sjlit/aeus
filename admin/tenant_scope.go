package admin

import (
	"context"
	"reflect"
	"sync"

	"github.com/sjlit/aeus/admin/middleware"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// tenantBypassKey marks operations that intentionally span tenants:
// Seed/Setup bootstrapping, cross-tenant admin tools, and test fixtures.
type tenantBypassKey struct{}

// WithoutTenant returns a context that disables tenant scoping for the
// DB operations using it. Use it only where cross-tenant access is the
// explicit intent; ordinary request paths must never carry it.
func WithoutTenant(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantBypassKey{}, true)
}

func tenantBypassed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(tenantBypassKey{}).(bool)
	return v
}

// __tenantSchemaCache memoizes the tenant-column lookup against
// *schema.Schema pointers. GORM interns a single *schema.Schema per
// (db, model-type) for the lifetime of the process, so the cache
// holds at most one entry per registered tenant-aware model and
// the hot path becomes a single Load.
var __tenantSchemaCache sync.Map // map[*schema.Schema]bool

// Callback names are part of the package's internal contract: Seed uses
// the query callback's presence to decide whether per-tenant schema
// clones are needed.
const (
	tenantQueryCallback  = "aeus:tenant:query"
	tenantUpdateCallback = "aeus:tenant:update"
	tenantDeleteCallback = "aeus:tenant:delete"
	tenantCreateCallback = "aeus:tenant:create"
)

// installTenantScope wires GORM callbacks so every read/update/delete on
// a model that carries the `tenant_id` column is automatically scoped
// to the tenant id produced by the resolver, and every create on such a
// model has its `TenantID` field forced to the resolver value.
//
// The callbacks run BEFORE the SQL is generated (Before("gorm:query"),
// Before("gorm:update"), Before("gorm:delete")) so the WHERE clause is
// merged into the same compiled statement. For Create, we run
// Before("gorm:create") which fires after the per-model BeforeCreate
// hooks. The resolver wins over any tenant_id present in the request:
// otherwise a caller could write into another tenant by supplying the
// field in the JSON body.
//
// The resolver is invoked against the *operation's* ctx on every DB
// call — there's no separate "store tenant on ctx" middleware to
// install. Tenant scoping is opt-in (admin.WithMultiTenant(true)); when
// enabled, the JWT middleware writes claims on ctx and the callback's
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
	query.Before("gorm:query").Register(tenantQueryCallback, func(tx *gorm.DB) {
		applyTenantScope(tx, resolver)
	})

	update := db.Callback().Update()
	update.Before("gorm:update").Register(tenantUpdateCallback, func(tx *gorm.DB) {
		applyTenantScope(tx, resolver)
	})

	delete := db.Callback().Delete()
	delete.Before("gorm:delete").Register(tenantDeleteCallback, func(tx *gorm.DB) {
		applyTenantScope(tx, resolver)
	})

	create := db.Callback().Create()
	create.Before("gorm:create").Register(tenantCreateCallback, func(tx *gorm.DB) {
		if !hasTenantIDColumn(tx.Statement.Schema) || tenantBypassed(tx.Statement.Context) {
			return
		}
		tid := resolver(tx.Statement.Context)
		if tid == "" {
			return
		}
		setTenantID(tx.Statement.ReflectValue, tid)
	})
}

// tenantScopingEnabled reports whether installTenantScope has registered
// its query callback on db. Single-tenant deployments skip it, and Seed
// uses the same signal to avoid cloning schema metadata that no reader
// will ever filter by tenant.
func tenantScopingEnabled(db *gorm.DB) bool {
	return db != nil && db.Callback().Query().Get(tenantQueryCallback) != nil
}

// applyTenantScope is shared by Query / Update / Delete: it calls
// the resolver against the operation's ctx and, if a non-empty
// tenant id is produced, appends it as a WHERE clause.
//
// Models without a `tenant_id` column are skipped, and so are
// operations whose resolver returns "" (the explicit opt-out for
// super-admin tooling and the AuthService.Login RPC).
func applyTenantScope(tx *gorm.DB, resolver middleware.Resolver) {
	if !hasTenantIDColumn(tx.Statement.Schema) || tenantBypassed(tx.Statement.Context) {
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

// setTenantID walks rv (which may be a struct, a pointer to struct, or a
// slice of either) and forces every embedded TenantID field to tid. The
// resolver is the single source of truth for the active tenant: letting a
// request body win would allow a caller to write into another tenant by
// supplying a tenant_id that the generic REST decoder accepts. System
// paths (Seed/Setup) opt out with WithoutTenant; Login opts out when the
// resolver returns "" (the default claims resolver has no claims yet).
func setTenantID(rv reflect.Value, tid string) {
	if !rv.IsValid() {
		return
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			setTenantID(rv.Index(i), tid)
		}
	case reflect.Pointer, reflect.Interface:
		if rv.IsNil() {
			return
		}
		setTenantID(rv.Elem(), tid)
	case reflect.Struct:
		t := rv.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				setTenantID(rv.Field(i), tid)
				continue
			}
			if f.Name == "TenantID" && f.Type.Kind() == reflect.String {
				fv := rv.Field(i)
				if fv.CanSet() {
					fv.SetString(tid)
				}
			}
		}
	}
}
