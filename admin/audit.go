package admin

import (
	"context"
	"encoding/json"
	"sort"
	"unicode/utf8"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/infra/logger"
	"github.com/sjlit/rest/v3"
	"gorm.io/gorm"
)

// Operation audit: when Options.Audit.Enabled is true, Setup installs
// three process-global rest/v3 after-hooks (create / update / delete)
// that append one models.Audit row per successful REST write — for
// every resource registered through this admin Server AND for any
// resource other modules register afterwards, because rest/v3
// snapshots the global hook list at resource-construction time and
// Setup installs the hooks before its own getModels() loop runs.
//
// Writes are synchronous best-effort. The hooks already run OUTSIDE
// the business transaction (rest/v3 fires runAfterHooks only after
// Transaction returns) and inside safelog.SafeRun, so an audit insert
// neither extends business locking nor can crash the request; a failed
// insert is logged at Warn level and swallowed.

const (
	// Built-in exclusions, always active on top of Options.Audit.Excludes.
	_auditExcludeAudits    = "system/sys_audits"     // recursion guard: never audit the audit table itself
	_auditExcludeLoginLogs = "system/sys_login_logs" // login auditing owns its own LoginLogger pipeline

	_auditActionCreate = "create"
	_auditActionUpdate = "update"
	_auditActionDelete = "delete"

	// Column-size mirrors of models.Audit; rows are truncated to fit.
	_auditDataMaxBytes   = 10240 // size:10240
	_auditUIDMaxBytes    = 20    // size:20
	_auditNameMaxBytes   = 60    // size:60 (module / table)
	_auditTenantMaxBytes = 60    // type:char(60)
)

// installAuditHooks registers the global rest/v3 after-hooks exactly
// once per Server. Called from Setup when Audit.Enabled is true —
// BEFORE the getModels() loop so every resource built afterwards
// snapshots the hooks.
//
// Note the registry is process-global: enabling audit on any Server
// affects every rest/v3 resource created later in the process, even
// ones owned by other modules or by other Server instances in tests.
// Symmetrically, enabling it on TWO Servers in one process double-
// records shared resources — run exactly one audit-enabled Server per
// process.
func (s *Server) installAuditHooks() {
	s.auditHooksOnce.Do(func() {
		excludes := map[string]struct{}{
			_auditExcludeAudits:    {},
			_auditExcludeLoginLogs: {},
		}
		for _, e := range s.opts.Audit.Excludes {
			excludes[e] = struct{}{}
		}
		rec := &auditRecorder{db: s.opts.DB, log: s.opts.Logger, excludes: excludes}
		rest.RegisterAfterCreate(rec.afterCreate)
		rest.RegisterAfterUpdate(rec.afterUpdate)
		rest.RegisterAfterDelete(rec.afterDelete)
		s.opts.Logger.Info(context.Background(), "operation audit hooks enabled",
			"excludes", keysOf(excludes))
	})
}

// auditRecorder carries the write path for the audit hooks. One
// instance is bound to the enabling Server's DB handle and logger.
type auditRecorder struct {
	db       *gorm.DB
	log      logger.Logger
	excludes map[string]struct{}
}

func (r *auditRecorder) afterCreate(ctx context.Context, _ *gorm.DB, _ any, diff []*rest.DiffAttr) {
	r.record(ctx, rest.RuntimeScopeFromContext(ctx), _auditActionCreate, diff, nil)
}

func (r *auditRecorder) afterUpdate(ctx context.Context, _ *gorm.DB, _ any, diff []*rest.DiffAttr) {
	r.record(ctx, rest.RuntimeScopeFromContext(ctx), _auditActionUpdate, diff, nil)
}

func (r *auditRecorder) afterDelete(ctx context.Context, _ *gorm.DB, model any) {
	r.record(ctx, rest.RuntimeScopeFromContext(ctx), _auditActionDelete, nil, model)
}

// record assembles and inserts one sys_audits row. Every failure mode
// is a Warn log, never an error: audit must not take the business
// response down with it.
//
// The insert runs on context.WithoutCancel(ctx): the hook fires after
// the business transaction has committed but before the HTTP response
// is written, so a client disconnect that cancels the request ctx must
// not silently abort the audit row.
func (r *auditRecorder) record(ctx context.Context, scope *rest.RuntimeScope, action string, diff []*rest.DiffAttr, deleted any) {
	if !r.shouldRecord(scope, action, diff) {
		return
	}
	row := models.Audit{
		TenantModel: models.TenantModel{
			TenantID: truncateUTF8(scope.TenantID, _auditTenantMaxBytes),
		},
		UID:    truncateUTF8(scope.User, _auditUIDMaxBytes),
		Action: action,
		Module: truncateUTF8(scope.ModuleName, _auditNameMaxBytes),
		Table:  truncateUTF8(scope.TableName, _auditNameMaxBytes),
		Data:   auditData(action, diff, deleted),
	}
	if err := r.db.WithContext(context.WithoutCancel(ctx)).Create(&row).Error; err != nil {
		r.log.Warn(ctx, "write operation audit failed",
			"error", err.Error(),
			"action", action,
			"module", scope.ModuleName,
			"table", scope.TableName,
		)
	}
}

// shouldRecord filters out everything that must not produce a row:
// requests without attribution info (no RuntimeScope module/table —
// e.g. writes issued outside the REST layer), excluded models, and
// updates that changed nothing (rest/v3 hands back an empty diff).
func (r *auditRecorder) shouldRecord(scope *rest.RuntimeScope, action string, diff []*rest.DiffAttr) bool {
	if scope == nil || scope.ModuleName == "" || scope.TableName == "" {
		return false
	}
	if _, ok := r.excludes[scope.ModuleName+"/"+scope.TableName]; ok {
		return false
	}
	if action == _auditActionUpdate && len(diff) == 0 {
		return false
	}
	return true
}

// auditData serializes the payload for the Data column:
//
//   - create/update: the DiffAttr slice as JSON — create carries
//     previous=null full values, update only the changed fields;
//   - delete: {"id":<pk>} extracted from the pre-delete snapshot
//     rest/v3 loaded. The full row is deliberately NOT recorded:
//     marshaling whole models would copy sensitive columns (e.g.
//     User.Password bcrypt hashes) into sys_audits.data.
func auditData(action string, diff []*rest.DiffAttr, deleted any) string {
	var (
		b   []byte
		err error
	)
	switch action {
	case _auditActionDelete:
		payload := map[string]any{"id": deletePrimaryKey(deleted)}
		b, err = json.Marshal(payload)
	default:
		b, err = json.Marshal(diff)
	}
	if err != nil || b == nil {
		return ""
	}
	return truncateUTF8(string(b), _auditDataMaxBytes)
}

// deletePrimaryKey extracts the primary key from the model snapshot
// that rest/v3's Delete path loads before issuing tx.Delete. The
// generic JSON round-trip keeps this independent of each model's PK
// field name/type — BaseModel.ID serializes as "id".
func deletePrimaryKey(model any) any {
	if model == nil {
		return nil
	}
	b, err := json.Marshal(model)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m["id"]
}

// truncateUTF8 cuts s to at most max BYTES without splitting a rune,
// mirroring how gorm column sizes bound storage while keeping the
// stored JSON valid UTF-8.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
