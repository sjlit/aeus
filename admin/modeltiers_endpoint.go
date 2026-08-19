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

// ModelTiersEndpointPath is the route mounted by RegisterModelTiersEndpoint.
// It returns the hierarchical option list for a (module, table) pair —
// the same shape rest.ModelTiers emits, with each node carrying a
// human-readable Label, a machine-readable Value, and a Children slice
// of nested nodes — so the front-end can drive tree-style pickers
// (cascading dropdowns, parent selectors) without bespoke per-table
// RPCs.
//
// Path:    GET /rest/model-tiers/:module/:table
//
//	?parent=<column>    (required) column whose value links a child to
//	                    its parent (e.g. Menu.Parent)
//	?label=<column>     (required) column whose value populates Label
//	?value=<column>     (required) column whose value populates Value
//	?valueType=<type>   (optional) one of string|int|int64|uint|uint64
//	                    default "string" — selects the generic
//	                    instantiation of rest.ModelTiers used to read
//	                    parent/value columns.
//	?tenant=<id>        (optional) explicit tenant override; defaults to
//	                    s.opts.TenantResolver(ctx). Pass "" to query
//	                    un-scoped (cross-tenant tooling).
//
// Auth:    must be mounted behind the JWT middleware (no allowlist
//
//	entry). All callers must present a valid bearer token.
//
// Returns: 200 with {code:0, message:"", data:[{label,value,parent,
//	children: [...]}, ...]} on success; 200 with
//	{code:4001|4004, message:"...", data:null} on the matching
//	error path. HTTP status stays 200 in every case — the
//	envelope's code field is the contract the front-end branches on.
const ModelTiersEndpointPath = "/rest/model-tiers/:module/:table"

// RegisterModelTiersEndpoint wires GET ModelTiersEndpointPath onto
// s.opts.Router and returns the registered handler for direct
// invocation.
//
// Mirrors RegisterModelTypesEndpoint's structure: takes *Server (for
// s.modelsByModuleTable lookup), validates Router/DB pre-conditions,
// mounts the route.  See that function's comment for the why-we-take-
// Server-not-Options rationale.
//
// The handler shares parseModuleTablePath with the model-types
// endpoint (the only difference between the two URLs is the literal
// "model-types" vs. "model-tiers" segment) so the parsing contract
// stays consistent.
func RegisterModelTiersEndpoint(s *Server) (http.HandlerFunc, error) {
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
		module, table, ok := parseModuleTablePath(r.URL.Path, "model-tiers")
		if !ok {
			writeEnvelope(w, int(errs.CodeInvalid), "module and table required", nil)
			return
		}
		parent, label, value, valueType, err := parseModelTierQuery(r)
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
		items, err := queryModelTiers(r.Context(), s.opts.DB, model, tenant, parent, label, value, valueType)
		if err != nil {
			writeEnvelope(w, int(errs.CodeInvalid), err.Error(), nil)
			return
		}
		writeEnvelope(w, int(errs.CodeOK), "", items)
	}
	s.opts.Router.Handle(http.MethodGet, ModelTiersEndpointPath, h)
	return h, nil
}

// parseModelTierQuery reads ?parent, ?label, ?value, ?valueType from r
// and validates them.  All three column names are required —
// rest.ModelTiers has no meaningful default for any — and valueType is
// restricted to the same set used by parseModelTypeQuery so the two
// endpoints share a single valueType vocabulary.
func parseModelTierQuery(r *http.Request) (parent, label, value, valueType string, err error) {
	q := r.URL.Query()
	parent = strings.TrimSpace(q.Get("parent"))
	label = strings.TrimSpace(q.Get("label"))
	value = strings.TrimSpace(q.Get("value"))
	if parent == "" {
		return "", "", "", "", fmt.Errorf("query ?parent is required")
	}
	if label == "" {
		return "", "", "", "", fmt.Errorf("query ?label is required")
	}
	if value == "" {
		return "", "", "", "", fmt.Errorf("query ?value is required")
	}
	valueType = strings.TrimSpace(q.Get("valueType"))
	if valueType == "" {
		valueType = "string"
	}
	switch valueType {
	case "string", "int", "int64", "uint", "uint64":
	default:
		return "", "", "", "", fmt.Errorf("unsupported valueType %q (allowed: string, int, int64, uint, uint64)", valueType)
	}
	return parent, label, value, valueType, nil
}

// queryModelTiers dispatches rest.ModelTiers by the requested value
// type.  Each branch returns []*rest.TierValue[T], which json.Marshal
// happily turns into [{label,value,parent,children:[...]}, ...]
// regardless of T.  The recursive children are built by
// rest.ModelTiers itself, so the only difference between branches is
// how each individual (parent, value) cell is read.
//
// The string branch is the common path (Menu.Component); the numeric
// branches mirror queryModelTypes for integer-id models.
func queryModelTiers(ctx context.Context, db *gorm.DB, model any, tenant, parent, label, value, valueType string) (any, error) {
	switch valueType {
	case "string":
		return rest.ModelTiers[string](ctx, db, model, tenant, parent, label, value)
	case "int":
		return rest.ModelTiers[int](ctx, db, model, tenant, parent, label, value)
	case "int64":
		return rest.ModelTiers[int64](ctx, db, model, tenant, parent, label, value)
	case "uint":
		return rest.ModelTiers[uint](ctx, db, model, tenant, parent, label, value)
	case "uint64":
		return rest.ModelTiers[uint64](ctx, db, model, tenant, parent, label, value)
	}
	// Defensive: parseModelTierQuery already filters every other value,
	// so this branch is unreachable in practice.
	return nil, fmt.Errorf("unsupported valueType %q", valueType)
}
