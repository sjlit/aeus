package admin

import (
	"errors"

	"github.com/sjlit/aeus/pkg/errs"
)

var (
	// ErrHTTPRequired is returned by Server.Setup / RegisterModel when no
	// rest.Router has been configured via WithRouter.
	ErrHTTPRequired = errors.New("http server required")
	// ErrDBRequired is returned by Server.Setup / RegisterModel /
	// RegisterSchemaEndpoint when no *gorm.DB has been configured via
	// WithDB.
	ErrDBRequired = errors.New("db required")
	// ErrRouterRequired is returned by RegisterSchemaEndpoint when the
	// supplied *Options lacks a Router.
	ErrRouterRequired = errors.New("router required")
	// ErrTenantRequired is returned by tenant-scoped GORM callbacks when
	// the operation's context has neither a tenant claim nor an explicit
	// WithoutTenant marker.
	ErrTenantRequired = errs.New(errs.CodeAccessDenied, "tenant context required")
)
