package admin

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/rest/v3"
	"gorm.io/gorm"
)

// ModelTypesEndpointPath is the route mounted by RegisterModelTypesEndpoint.
// It returns the option list for a (module, table) pair — one entry per
// row in the underlying table, each carrying a human-readable Label and
// a machine-readable Value — so the front-end (@nobla/rest-ui) can
// drive select / dropdown pickers without bespoke per-table RPCs.
//
// Path:    GET /rest/model-types/:module/:table
//
//	?label=<column>     (required) column whose value populates Label
//	?value=<column>     (required) column whose value populates Value
//	?valueType=<type>   (optional) one of string|int|int64|uint|uint64
//	                    default "string" — selects the generic
//	                    instantiation of rest.ModelTypes used to read
//	                    the value column.
//	?tenant=<id>        (optional) explicit tenant override; defaults to
//	                    s.opts.TenantResolver(ctx). Pass "" to query
//	                    un-scoped (cross-tenant tooling).
//
// Auth:    must be mounted behind the JWT middleware (no allowlist
//
//	entry). All callers must present a valid bearer token.
//
// Returns: 200 with {code:0, message:"", data:[{label,value}, ...]}
//	on success; 200 with {code:4001|4004, message:"...", data:null}
//	on the matching error path. HTTP status stays 200 in every
//	case — the envelope's code field is the contract the front-end
//	branches on.
const ModelTypesEndpointPath = "/rest/model-types/:module/:table"

// RegisterModelTypesEndpoint wires GET ModelTypesEndpointPath onto
// s.opts.Router and returns the registered handler for direct
// invocation.
//
// The function takes *Server (not *Options) because it needs to read
// s.modelsByModuleTable, populated by registerModel — the same code
// path that backs Setup's getModels() loop and the public
// RegisterModel.  Pre-condition failures (missing Router / DB) surface
// here instead of at first request time, matching the
// RegisterSchemaEndpoint contract.
//
// The handler reads path params :module and :table from r.URL.Path via
// parseModuleTablePath, which mirrors parseSchemaPath's split-and-count
// contract — both endpoints live under /rest/ so a single helper keeps
// the parsing rules consistent.
func RegisterModelTypesEndpoint(s *Server) (http.HandlerFunc, error) {
	if s == nil || s.opts == nil {
		return nil, ErrRouterRequired
	}
	if s.opts.Router == nil {
		return nil, ErrRouterRequired
	}
	if s.opts.DB == nil {
		return nil, ErrDBRequired
	}
	h := func(w http.ResponseWriter, r *http.Request) {
		module, table, ok := parseModuleTablePath(r.URL.Path, "model-types")
		if !ok {
			writeEnvelope(w, int(errs.CodeInvalid), "module and table required", nil)
			return
		}
		label, value, valueType, err := parseModelTypeQuery(r)
		if err != nil {
			writeEnvelope(w, int(errs.CodeInvalid), err.Error(), nil)
			return
		}
		model, ok := s.lookupModel(module, table)
		if !ok {
			writeEnvelope(w, int(errs.CodeNotFound),
				"model not registered for module="+module+", table="+table, nil)
			return
		}
		tenant := strings.TrimSpace(r.URL.Query().Get("tenant"))
		if tenant == "" && s.opts.TenantResolver != nil {
			tenant = s.opts.TenantResolver(r.Context())
		}
		items, err := queryModelTypes(r.Context(), s.opts.DB, model, tenant, label, value, valueType)
		if err != nil {
			writeEnvelope(w, int(errs.CodeInvalid), err.Error(), nil)
			return
		}
		writeEnvelope(w, int(errs.CodeOK), "", items)
	}
	s.opts.Router.Handle(http.MethodGet, ModelTypesEndpointPath, h)
	return h, nil
}

// parseModuleTablePath extracts the :module and :table segments from a
// request path like "/rest/<segment>/system/sys_users".  Returns
// ok=false when the path doesn't have the expected 5 non-empty segments
// (rest / <segment> / module / table) under a leading slash, or when
// <segment> doesn't match the supplied wantSegment.  This is shared by
// both model-types and model-tiers — the only difference between the
// two URLs is the second segment.
//
// Trailing slashes are tolerated (matching parseSchemaPath's behavior);
// the function deliberately does not touch any router-internal state —
// rest.Router gives us only stdlib http.Request, and we mirror
// rest/v3's own path-param extraction pattern (resource.go:159-180).
func parseModuleTablePath(p, wantSegment string) (module, table string, ok bool) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) != 4 || parts[0] != "rest" || parts[1] != wantSegment {
		return "", "", false
	}
	if parts[2] == "" || parts[3] == "" {
		return "", "", false
	}
	return parts[2], parts[3], true
}

// parseModelTypeQuery reads ?label, ?value, ?valueType from r and
// validates them.  Both column names are required — rest.ModelTypes
// has no meaningful default for either — and valueType is restricted
// to the set of types we actually instantiate below; an unknown type
// short-circuits to a 4001 before we touch the database.
func parseModelTypeQuery(r *http.Request) (label, value, valueType string, err error) {
	q := r.URL.Query()
	label = strings.TrimSpace(q.Get("label"))
	value = strings.TrimSpace(q.Get("value"))
	if label == "" {
		return "", "", "", fmt.Errorf("query ?label is required")
	}
	if value == "" {
		return "", "", "", fmt.Errorf("query ?value is required")
	}
	valueType = strings.TrimSpace(q.Get("valueType"))
	if valueType == "" {
		valueType = "string"
	}
	switch valueType {
	case "string", "int", "int64", "uint", "uint64":
	default:
		return "", "", "", fmt.Errorf("unsupported valueType %q (allowed: string, int, int64, uint, uint64)", valueType)
	}
	return label, value, valueType, nil
}

// queryModelTypes dispatches rest.ModelTypes by the requested value
// type.  Each branch returns []*rest.TypeValue[T], which json.Marshal
// happily turns into [{"label":"...","value":...}, ...] regardless of
// T.  Errors are forwarded as-is (GORM errors are already
// human-readable).
//
// The string branch is the common path: most admin pickers submit a
// string code (Menu.Component, Role.Key).  The numeric branches exist
// for models whose natural id is an integer (sys_users.id, sys_roles.id)
// so the value round-trips losslessly to the front-end without a
// string<->int coercion step in every consumer.
func queryModelTypes(ctx context.Context, db *gorm.DB, model any, tenant, label, value, valueType string) (any, error) {
	switch valueType {
	case "string":
		return rest.ModelTypes[string](ctx, db, model, tenant, label, value)
	case "int":
		return rest.ModelTypes[int](ctx, db, model, tenant, label, value)
	case "int64":
		return rest.ModelTypes[int64](ctx, db, model, tenant, label, value)
	case "uint":
		return rest.ModelTypes[uint](ctx, db, model, tenant, label, value)
	case "uint64":
		return rest.ModelTypes[uint64](ctx, db, model, tenant, label, value)
	}
	// Defensive: parseModelTypeQuery already filters every other value,
	// so this branch is unreachable in practice.
	return nil, fmt.Errorf("unsupported valueType %q", valueType)
}

// lookupModel resolves a registered model instance by (module, table).
// Returns ok=false when the (module, table) pair has no registered
// model — the caller turns that into a 4004 envelope so the front-end
// can distinguish "URL typo" from "real DB error".
func (s *Server) lookupModel(module, table string) (any, bool) {
	if s == nil || s.modelsByModuleTable == nil {
		return nil, false
	}
	m, ok := s.modelsByModuleTable[module+"/"+table]
	return m, ok
}
