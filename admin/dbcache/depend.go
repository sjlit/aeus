package dbcache

import (
	"context"
	"fmt"
	"regexp"

	"gorm.io/gorm"
)

// CacheDependency supplies the version marker a Cacher compares against
// the marker captured when an entry was stored. A value change means
// the underlying data changed and cached entries must reload.
//
// Implementations must run their queries on tx, which carries the
// request context (the Cacher always passes WithContext'ed tx). The
// ctx argument is provided for non-SQL dependencies and to satisfy the
// contract explicitly.
type CacheDependency interface {
	GetValue(ctx context.Context, tx *gorm.DB) (value string, err error)
}

var (
	// validTableName bounds the table identifier to a bare name.
	validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	// validColumnExpr bounds the column expression to identifiers,
	// whitespace and function/aggregate punctuation — enough for
	// "MAX(updated_at)", nothing more.
	validColumnExpr = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_\s\(\),]*$`)
)

// SqlDependency is a CacheDependency whose marker is a scalar SQL
// result, typically MAX(updated_at) of the table the loader reads.
// Table and column are validated against the patterns above before use
// (they are developer-supplied literals, but a defense-in-depth check
// keeps a typo or a stray injection string from reaching the DB).
type SqlDependency struct {
	table         string
	column        string
	model         any
	condition     string
	conditionArgs []any
}

// DepOption configures a SqlDependency.
type DepOption func(*SqlDependency)

// WithTable sets the target table by name. Mutually exclusive with
// WithModel.
func WithTable(table string) DepOption {
	return func(o *SqlDependency) {
		o.table = table
	}
}

// WithColumn sets the scalar expression to select, e.g.
// "MAX(updated_at)". Required.
func WithColumn(column string) DepOption {
	return func(o *SqlDependency) {
		o.column = column
	}
}

// WithModel sets the target table via a GORM model. Mutually exclusive
// with WithTable.
func WithModel(model any) DepOption {
	return func(o *SqlDependency) {
		o.model = model
	}
}

// WithCondition narrows the marker query to a subset of rows, e.g.
// ("tenant_id = ?", tenantID). An empty condition is ignored.
func WithCondition(condition string, args ...any) DepOption {
	return func(o *SqlDependency) {
		o.condition = condition
		o.conditionArgs = args
	}
}

// NewSqlDependency builds a SqlDependency. The marker query is
// "SELECT <column> FROM <table|model> [WHERE <condition>]", returning a
// single scalar into a string.
func NewSqlDependency(cbs ...DepOption) *SqlDependency {
	d := &SqlDependency{}
	for _, cb := range cbs {
		cb(d)
	}
	return d
}

// GetValue runs the marker query on tx. The ctx parameter is carried
// in the interface for non-SQL dependencies; SqlDependency itself
// relies on tx already carrying the request context.
func (d *SqlDependency) GetValue(ctx context.Context, tx *gorm.DB) (value string, err error) {
	tx = tx.WithContext(ctx)
	if !validColumnExpr.MatchString(d.column) {
		return "", fmt.Errorf("invalid column expression: %q", d.column)
	}
	var q *gorm.DB
	switch {
	case d.table != "":
		if !validTableName.MatchString(d.table) {
			return "", fmt.Errorf("invalid table name: %q", d.table)
		}
		q = tx.Table(d.table)
	case d.model != nil:
		q = tx.Model(d.model)
	default:
		return "", fmt.Errorf("dbcache: SqlDependency requires WithTable or WithModel")
	}
	if d.condition != "" {
		q = q.Where(d.condition, d.conditionArgs...)
	}
	// Scan into *string: aggregates over an empty table return NULL
	// (e.g. MAX/SUM on a fresh install), which becomes "" instead of a
	// scan error.
	var v *string
	if err := q.Select(d.column).Scan(&v).Error; err != nil {
		return "", err
	}
	if v != nil {
		return *v, nil
	}
	return "", nil
}
