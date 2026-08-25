package admin

import (
	"github.com/sjlit/aeus/admin/middleware"
	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/rest/v3"
	"gorm.io/gorm"
)

// AuditOptions configures the operation-audit feature installed by
// Server.Setup (see audit.go). Enabled flips on the process-global
// rest/v3 after-hooks that append one models.Audit row per successful
// REST create/update/delete; Excludes suppresses recording for
// specific "<module>/<table>" pairs on top of the built-in defaults
// ("system/sys_audits" — recursion guard — and "system/sys_login_logs",
// which owns its own LoginLogger pipeline).
type AuditOptions struct {
	Enabled  bool
	Excludes []string
}

type (
	// Options holds the dependencies Server wires the admin resources
	// onto. It is built by New via the With* Option functions.
	Options struct {
		DB             *gorm.DB
		Responder      rest.Responder
		EnabledOpenAPI bool
		// TenantResolver derives the tenant id that scopes every GORM
		// operation on a TenantModel row. Server.Setup installs the
		// callback once at boot time, then reads this field once per
		// DB op. nil is normalized to middleware.FromClaimsResolver,
		// which reads *auth.Claims.TenantID off the ctx written by
		// middleware/auth.JWT.
		TenantResolver middleware.Resolver

		//
		Router rest.Router

		Logger logger.Logger

		// Audit controls the operation-audit hooks (audit.go). The
		// zero value keeps the feature off — Setup registers the
		// global rest/v3 after-hooks only when Audit.Enabled is true.
		Audit AuditOptions

		// UserResolve derives the caller's UID that lands in
		// RuntimeScope.User (rest/v3) and in every audit row's uid
		// column. It is pushed into ResourceConfig.UserResolve by
		// registerModel for every model registered through this
		// Server. nil is normalized to middleware.FromClaimsUserResolve,
		// which reads *auth.Claims.UID off the ctx written by
		// middleware/auth.JWT.
		UserResolve rest.ResolveUserFunc

		// VueOutputDir is the directory under which auto-generated
		// Vue Index.vue files are written when registerModel sees a
		// model with ModuleName + TableName set. The full path of a
		// generated file is VueOutputDir / <ModuleName> / <Singular> /
		// Index.vue — VueOutputDir itself is the conventional `views/`
		// directory in Vite projects (i.e. the level immediately above
		// the per-module directories). Singular comes from rest/v3's
		// Naming (gorm.Tabler + the package inflector), mirroring
		// deriveViewPath so the generated view lines up with the
		// sys_menus.view_path row written by the same registerModel
		// call.
		//
		// Set via WithVueOutputDir. An empty value disables Vue
		// generation across all registered models — the per-call
		// RegisterModel option can still opt a single model in (or
		// override the per-call path) independently of this field.
		//
		// Generation is best-effort: a write failure is logged at Warn
		// level and never blocks the menu / permission inserts, because
		// the Vue template lives on the front-end side and isn't part
		// of the server's correctness contract.
		VueOutputDir string
	}

	// Option mutates Options.
	Option func(*Options)
)

func newOptions(opts ...Option) *Options {
	options := &Options{
		Responder: newResponder(),
		Logger:    logger.Default(),
	}
	for _, o := range opts {
		o(options)
	}
	if options.TenantResolver == nil {
		options.TenantResolver = middleware.FromClaimsResolver
	}
	if options.UserResolve == nil {
		options.UserResolve = middleware.FromClaimsUserResolve
	}
	return options
}

// WithResponder overrides the response envelope writer used by every
// REST resource. Defaults to the internal admin responder.
func WithResponder(responder rest.Responder) Option {
	return func(o *Options) {
		o.Responder = responder
	}
}

// WithOpenAPI toggles OpenAPI JSON generation on the registered
// resources.
func WithOpenAPI(enabled bool) Option {
	return func(o *Options) {
		o.EnabledOpenAPI = enabled
	}
}

func WithLogger(logger logger.Logger) Option {
	return func(o *Options) {
		o.Logger = logger
	}
}

// WithDB sets the gorm handle used for AutoMigrate, tenant-scope
// callbacks, and the auto-inserted menu / permission rows.
func WithDB(db *gorm.DB) Option {
	return func(o *Options) {
		o.DB = db
	}
}

// WithTenantResolver overrides the default tenant resolver.
//
// # Contract
//
// The resolver is called once per DB operation by the GORM callbacks
// installed in Server.Setup, with the request ctx that the caller
// passed to db.WithContext. It must return:
//
//   - a non-empty tenant id, in which case every read/update/delete
//     on a TenantModel row is automatically scoped to that id, and
//     every create on such a model has its TenantID field backfilled
//     from the resolver when the caller left it blank;
//   - "", in which case no scope is applied — the operation sees
//     every tenant's rows. This is the opt-out path for super-admin
//     tooling, cross-tenant reporting, and the AuthService.Login
//     RPC, which by design runs before any tenant is known.
//
// The resolver MUST be cheap and MUST NOT panic on a missing JWT —
// a malformed request just yields "" and the operation proceeds
// un-scoped; the resolver itself remains the gatekeeper for
// authentication decisions.
//
// # Default
//
// If WithTenantResolver is never called (or called with nil), the
// resolver is middleware.FromClaimsResolver, which returns
// *auth.Claims.TenantID. Operations invoked without JWT claims
// therefore get "" and run un-scoped.
//
// # Typical override
//
// Super-admin tooling wants to honor an explicit X-Tenant-Id header
// when present, but fall back to the JWT tenant otherwise:
//
//	admin.WithTenantResolver(func(ctx context.Context) string {
//	    if h := metadata.Get(ctx, "X-Tenant-Id"); h != "" {
//	        return h
//	    }
//	    return middleware.FromClaimsResolver(ctx)
//	})
//
// Note: this trusts the X-Tenant-Id header blindly; the caller is
// responsible for restricting that header to trusted roles upstream
// (e.g. an admin-only middleware on the super-admin routes).
//
// # See also
//
//   - middleware.Resolver — the function type itself;
//   - middleware.FromClaimsResolver — the default;
//   - admin.Server.Setup — where the callback is installed.
func WithTenantResolver(r middleware.Resolver) Option {
	return func(o *Options) {
		o.TenantResolver = r
	}
}

// WithRouter sets the HTTP router the REST resources are mounted on.
// Required — Server.Setup / RegisterModel fail with ErrHTTPRequired
// without it.
func WithRouter(router rest.Router) Option {
	return func(o *Options) {
		o.Router = router
	}
}

// WithVueOutputDir sets the base directory into which registerModel
// writes an auto-generated Index.vue per model. Pass an empty string
// to disable generation for the entire Server.
//
// The directory should be the conventional Vite `views/` directory
// (the level immediately above the per-module directories — e.g.
// `<repo>/web/src/views`). The generator appends `<module>/<singular>/Index.vue`
// using rest/v3's resolved Singular (see deriveViewPath); a model
// whose ModuleName or Singular can't be resolved is silently skipped
// since the matching sys_menus row can't be written either.
//
// A best-effort write failure is logged at Warn level and does not
// abort the menu / permission inserts. Re-running Setup is also safe
// — the generator skips files that already exist, so manual edits to
// the generated template are preserved across restarts.
func WithVueOutputDir(dir string) Option {
	return func(o *Options) {
		o.VueOutputDir = dir
	}
}

// WithAudit enables (or disables) the operation-audit hooks installed
// by Server.Setup. When enabled, Setup registers process-global
// rest/v3 after-hooks that append one models.Audit row per successful
// REST create/update/delete — for every resource registered through
// admin AND for resources any other module registers afterwards, as
// long as Setup ran first (rest/v3 snapshots global hooks at resource
// construction time).
//
// Writes are synchronous best-effort: a failed audit insert is logged
// at Warn level and never affects the business response.
func WithAudit(enabled bool) Option {
	return func(o *Options) {
		o.Audit.Enabled = enabled
	}
}

// WithAuditExcludes appends "<module>/<table>" pairs to the audit
// exclusion list. The built-in defaults ("system/sys_audits" and
// "system/sys_login_logs") always apply; entries added here suppress
// recording for additional models. Only meaningful when audit is
// enabled via WithAudit(true).
func WithAuditExcludes(moduleTable ...string) Option {
	return func(o *Options) {
		o.Audit.Excludes = append(o.Audit.Excludes, moduleTable...)
	}
}

// WithUserResolve overrides how the caller's UID is derived for
// RuntimeScope.User and audit rows. The default resolver reads
// *auth.Claims.UID off the JWT ctx; pass a custom func when callers
// authenticate differently.
//
// # Error contract
//
// rest/v3 invokes the resolver on EVERY create / update / delete
// request before the write executes, and a non-nil error aborts the
// operation with an "unavailable" response. Return (uid, nil) for the
// normal path and ("", nil) when no identity is present — reserve
// errors for identities that MUST reject the request outright.
func WithUserResolve(fn rest.ResolveUserFunc) Option {
	return func(o *Options) {
		o.UserResolve = fn
	}
}
