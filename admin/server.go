package admin

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/rest/v3"
	"github.com/sjlit/rest/v3/formats"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

// Server wires the admin domain's REST resources onto a rest.Router
// (typically *transport/http.Server, but anything implementing the
// single-method Handle interface is acceptable).
//
// AuthService is intentionally NOT registered by Setup: applications
// build it themselves with `service.NewAuthService(...)` and register
// the resulting AuthService on the same router via
// `pb.RegisterAuthServiceRouter(router, authSvc)`. This keeps the JWT
// secret / TokenStore / TTL choices in the application's hands and
// stops admin from baking server-wide auth defaults.
//
// The JWT middleware (middleware/auth.JWT) is the application's
// responsibility too — admin only ships the resource registrations.
type Server struct {
	opts *Options
}

// New builds a Server from the supplied options.
func New(opts ...Option) *Server {
	return &Server{opts: newOptions(opts...)}
}

func (s *Server) getModels() []any {
	return []any{
		&models.Menu{},
		&models.User{},
		&models.LoginLog{},
		&models.Department{},
		&models.Role{},
		// The per-tenant junction must be in getModels() so rest/v3's
		// AutoMigrate (inside NewResourceWithOptions) creates
		// sys_role_permissions on fresh environments. Without it, the
		// login flow (RolePermission.MenuPermissionDatas) fails with
		// "no such table: sys_role_permissions".
		&models.RolePermission{},
		&models.Permission{},
		&models.Audit{},
		&models.Tenant{},
	}
}

// RegisterModel wires an application model onto the http server's REST
// router.  Callers typically run Setup once at boot, then call
// RegisterModel for each application-specific model they want exposed
// after Setup's built-in getModels() loop has run.
//
// The model's table is auto-migrated by rest/v3; if the model also
// implements MenuProvider, a sys_menus row is auto-inserted with
// Component / Uri derived from ModuleName + TableName (when blank in
// MenuEntry).  This keeps the auto-menu behaviour uniform for
// built-in models wired in from Setup's getModels() loop AND for
// application models registered later via RegisterModel — the
// per-model insert is what makes the feature composable.
//
// RegisterModel must NOT be called before Setup has run: rest/v3 needs
// the schema meta-table and tenant-scope callbacks installed by Setup
// to wire routes correctly.
//
// Returns ErrHTTPRequired or ErrDBRequired if Setup's preconditions
// haven't been met (Setup itself returns the same).
//
// External callers that batch multiple RegisterModel calls and need
// orphan-parent validation should run validateMenuParentsRef once at
// the end of their batch.
func (s *Server) RegisterModel(model any) error {
	if s.opts.Router == nil {
		return ErrHTTPRequired
	}
	if s.opts.DB == nil {
		return ErrDBRequired
	}
	resourceDB := s.opts.DB.Session(&gorm.Session{NewDB: true, Initialized: true})
	return s.registerModel(model, resourceDB)
}

// registerModel is the per-model implementation behind RegisterModel
// and Setup's getModels() loop.  resourceDB must be a session with a
// statement detached from s.opts.DB.Statement: rest/v3's NewModel
// parses each model through `Session(&gorm.Session{NewDB: true})
// .Statement.Parse(...)`, and gorm (v1.31.1) only swaps the session's
// statement when Context / PrepareStmt / SkipHooks / Initialized is
// set — otherwise it keeps pointing at the caller's root statement.
// Parsing through the root statement would leave a stale Schema (last
// model) and Table (first model) on s.opts.DB.Statement; every later
// op built from s.opts.DB — e.g. `db.WithContext(ctx).Transaction(...)`
// in RoleService.ReplaceRolePermissions — clones that stale
// statement, sees Schema != nil, skips re-parsing and queries the
// wrong table.  RegisterModel hides this plumbing behind the public
// surface; Setup uses the same code path via the same wrapper.
//
// The tenant resolver, when set, is also pushed into rest/v3's
// ResourceConfig.TenantResolve so the Search / Export query paths
// automatically append `tenant_id = ?` to their WHERE clauses —
// matching the GORM-callback layer installed in Setup for every other
// DB op.  This is gated on the model actually declaring a tenant_id
// column (modelHasTenantIDColumn): rest/v3 appends the filter
// unconditionally, which would break Search/Export on global models
// (Menu, Permission, Tenant).
//
// When the model implements MenuProvider, registerModel also
// auto-creates the corresponding sys_menus row.  sys_menus is migrated
// lazily on the first MenuProvider so external callers don't have to
// ensure the table exists themselves (AutoMigrate is a no-op on
// already-migrated tables).
//
// Every registered model — MenuProvider or not — also gets one
// sys_permissions row per exposed scenario (create / update /
// delete / search / detail / export, by default; or whatever the
// model declares via rest.ScenarioProvider).  The auto-permission
// catalog is keyed on "<METHOD> <URI>", so enforcement middleware
// (future work) can match request method+path directly.  Like the
// menu insert, this runs through s.opts.DB (root) — see
// ensurePermissionRows for why the detached resourceDB is wrong here.
//
// The menu insert is run through s.opts.DB (root), not resourceDB:
// the detached session would otherwise leave db.Create returning
// gorm.ErrRecordNotFound on auto-default-valued columns (e.g.
// UpdatedAt autoUpdateTime) because the session's statement still
// carries the last parsed model's schema.  Plain data inserts don't
// need the detached session.
func (s *Server) registerModel(model any, resourceDB *gorm.DB) error {
	cfg := rest.ResourceConfig{
		Router:    s.opts.Router,
		Responder: s.opts.Responder,
		Formatter: formats.DefaultFormatter(),
	}
	if s.opts.TenantResolver != nil && modelHasTenantIDColumn(resourceDB, model) {
		resolver := s.opts.TenantResolver
		cfg.TenantResolve = func(ctx context.Context, _ *http.Request) (string, error) {
			return resolver(ctx), nil
		}
	}
	resource, err := rest.NewResourceWithOptions(model, cfg,
		rest.WithDB(resourceDB),
		rest.WithOpenAPI(s.opts.EnabledOpenAPI),
	)
	if err != nil {
		return err
	}
	if p, ok := model.(models.MenuProvider); ok {
		// Migrate sys_menus on a CLEAN detached session: resourceDB's
		// statement was left pointing at the last model rest/v3 parsed
		// (see the session-caching note above).  gorm's migrator
		// pre-seeds Table from the statement but re-parses the Schema,
		// so a polluted statement makes AutoMigrate(Menu) compare
		// Menu's fields against the wrong table — when that table's id
		// type differs from Menu's uint (e.g. Tenant's char(60) id),
		// the mismatch triggers a broken ALTER TABLE rebuild.
		if err := s.opts.DB.Session(&gorm.Session{NewDB: true, Initialized: true}).AutoMigrate(&models.Menu{}); err != nil {
			return fmt.Errorf("migrate sys_menus: %w", err)
		}
		spec := s.fillDerivedSpec(resource, p.MenuEntry())
		if _, err := s.ensureMenuRow(s.opts.DB, spec); err != nil {
			return fmt.Errorf("auto-create menu for %T: %w", model, err)
		}
	}
	if err := s.ensurePermissionRows(s.opts.DB, resource); err != nil {
		return err
	}
	return nil
}

// Setup installs tenant scoping and registers every built-in model on
// the configured router: it migrates the schema meta-table, auto-creates
// menu and permission rows, validates menu parents, and mounts the
// schema endpoint.
func (s *Server) Setup(ctx context.Context) (err error) {
	if s.opts.Router == nil {
		return ErrHTTPRequired
	}
	if s.opts.DB == nil {
		return ErrDBRequired
	}
	// Install tenant scope BEFORE any model is registered so every
	// CRUD route that rest/v3 wires up is already scoped when the
	// first request lands. AutoMigrate itself does NOT route through
	// the Create/Query/Update/Delete callback chains — it issues raw
	// DDL — so order is irrelevant to the schema, only to the first
	// CRUD call.
	installTenantScope(s.opts.DB, s.opts.TenantResolver)
	// schema.Schema{} is the meta-table that rest/v3 queries for resource
	// metadata; it must exist before RegisterModel can succeed. rest/v3
	// automatically runs db.AutoMigrate on each registered model inside
	// NewResourceWithOptions, so we do NOT migrate business models here.
	if err = s.opts.DB.AutoMigrate(&schema.Schema{}); err != nil {
		return fmt.Errorf("migrate schema meta table: %w", err)
	}

	for _, m := range s.getModels() {
		if err = s.RegisterModel(m); err != nil {
			return err
		}
	}
	// Each RegisterModel call above auto-inserts the corresponding
	// sys_menus row when the model implements MenuProvider.  Now the
	// database is the ground truth: scan for rows whose Parent
	// references a non-existent Component and fail fast with the
	// dangling references listed.  This also catches orphans inserted
	// by external RegisterModel callers, since every row in sys_menus
	// is scanned regardless of origin.
	if err = s.validateMenuParentsRef(s.opts.DB); err != nil {
		return fmt.Errorf("auto-create menus: %w", err)
	}

	// Mount GET /schema/:module/:table after the built-in models are
	// registered so the schema meta-table is already populated for the
	// front-end (@nobla/rest-ui) to fetch column metadata for the CRUD
	// pages. Pre-condition failures (missing Router / DB) surface here
	// instead of at first request time.
	if _, err = RegisterSchemaEndpoint(s.opts); err != nil {
		return fmt.Errorf("register schema endpoint: %w", err)
	}
	return nil
}
