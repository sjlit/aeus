package admin

import (
	"github.com/sjlit/aeus/admin/middleware"
	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/rest/v3"
	"gorm.io/gorm"
)

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
