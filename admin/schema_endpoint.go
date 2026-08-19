package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

// SchemaEndpointPath is the route mounted by RegisterSchemaEndpoint. It
// returns the full rest/v3 schema metadata for a (module, table) pair as
// a JSON array wrapped in admin's standard {code, message, data}
// envelope. The frontend (@nobla/rest-ui) reads the JSON to drive
// automatic CRUD rendering.
//
// Path:    GET /schema/:module/:table
// Auth:    must be mounted behind the JWT middleware (no allowlist
//
//	entry). All callers must present a valid bearer token.
//
// Returns: 200 with {code:0, message:"", data:[...]} on success;
//
//	200 with {code:4001|4004|..., message:"...", data:null}
//	on the matching error path. The HTTP status stays 200 in
//	every case — the envelope's code field is the contract the
//	frontend branches on.
const SchemaEndpointPath = "/schema/:module/:table"

// RegisterSchemaEndpoint wires GET SchemaEndpointPath onto opts.Router
// and returns the registered handler for direct invocation.
//
// Returns ErrRouterRequired / ErrDBRequired when the supplied *Options
// is missing the corresponding field. Failing at boot time surfaces the
// misconfiguration before the first request lands.
//
// The handler reads path params :module and :table from r.URL.Path via
// parseSchemaPath (mirroring rest/v3's resource.go:159-180 path-param
// extraction — we register through the minimal rest.Router interface,
// which strips gin context, so stdlib-style path splitting is the only
// portable route).
func RegisterSchemaEndpoint(opts *Options) (http.HandlerFunc, error) {
	if opts == nil {
		return nil, ErrRouterRequired
	}
	if opts.Router == nil {
		return nil, ErrRouterRequired
	}
	if opts.DB == nil {
		return nil, ErrDBRequired
	}
	db := opts.DB
	h := func(w http.ResponseWriter, r *http.Request) {
		module, table, ok := parseSchemaPath(r.URL.Path)
		if !ok {
			writeEnvelope(w, int(errs.CodeInvalid), "module and table required", nil)
			return
		}
		items, err := querySchemas(r.Context(), db, module, table)
		if err != nil {
			writeEnvelope(w, int(errs.CodeInvalid), err.Error(), nil)
			return
		}
		if len(items) == 0 {
			writeEnvelope(w, int(errs.CodeNotFound),
				"schema not found for module="+module+", table="+table, nil)
			return
		}
		writeEnvelope(w, int(errs.CodeOK), "", items)
	}
	opts.Router.Handle(http.MethodGet, SchemaEndpointPath, h)
	return h, nil
}

// parseSchemaPath extracts the :module and :table segments from a
// request path like "/schema/system/sys_users". Returns ok=false when
// the path doesn't have exactly three non-empty segments under /schema/.
//
// Trailing slashes are tolerated. The function deliberately does not
// touch any router-internal state — rest.Router gives us only stdlib
// http.Request, and we mirror rest/v3's own path-param extraction
// pattern (resource.go:159-180).
func parseSchemaPath(p string) (module, table string, ok bool) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) != 3 || parts[0] != "schema" {
		return "", "", false
	}
	if parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// querySchemas pulls the full rest/v3 schema row set for a
// (module, table) pair. Returns an empty slice (not an error) when the
// table is unknown; the handler decides whether to surface that as a
// 4004.
//
// We use schema.GetSchemas (not GetVisibleSchemas) because the
// frontend needs every column regardless of scenario — each row carries
// its own scenarios/visible/readonly/disable metadata, which the
// renderer evaluates per-scenario at use time.
func querySchemas(ctx context.Context, db *gorm.DB, module, table string) ([]schema.Schema, error) {
	tx := db.Unscoped()
	return schema.GetSchemas(ctx, tx, module, table)
}
