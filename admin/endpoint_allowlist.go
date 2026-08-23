package admin

import (
	"context"
	"fmt"

	"github.com/sjlit/rest/v3/schema"
	"gorm.io/gorm"
)

// _readScenarios is the set of scenario strings that mark a column as
// readable through some GET path.  "view" is not a rest/v3 canonical
// scenario constant, but every built-in admin model uses it in
// scenarios tags, so it counts as a read indicator alongside the
// schema package's own constants.
var _readScenarios = []string{
	schema.ScenarioSearch,
	schema.ScenarioList,
	schema.ScenarioDetail,
	"view",
	schema.ScenarioExport,
}

// selectableColumnsDB returns the set of columns that may be referenced
// as label / value / parent inputs on the model-types and model-tiers
// endpoints, derived from the sys_schemas metadata Setup wrote for
// (module, table).
//
// A column qualifies only when BOTH hold:
//
//   - its format is not FormatPassword — password-shaped columns are
//     never picker material, even if an operator later widens their
//     scenarios;
//   - at least one read scenario is declared — write-only columns such
//     as User.Password (scenarios:"create") can never be exported
//     through these endpoints.
//
// The lookup fails closed: when sys_schemas has no rows for the pair
// (metadata missing or table never registered), an empty set with an
// explanatory error is returned instead of an implicit allow-all.
func selectableColumnsDB(ctx context.Context, db *gorm.DB, module, table string) (map[string]struct{}, error) {
	rows, err := querySchemas(ctx, db, module, table)
	if err != nil {
		return nil, fmt.Errorf("load sys_schemas for %s/%s: %w", module, table, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no schema metadata for %s/%s", module, table)
	}
	out := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.Column == "" || row.Format == schema.FormatPassword {
			continue
		}
		for _, sc := range _readScenarios {
			if row.Scenarios.Has(sc) {
				out[row.Column] = struct{}{}
				break
			}
		}
	}
	return out, nil
}

// requireSelectableColumns validates that every one of cols is inside
// the selectable allowlist for (module, table).  Returns a
// client-facing error naming the first offending column so the handler
// can surface it as 1001 Invalid.
func requireSelectableColumns(ctx context.Context, db *gorm.DB, module, table string, cols ...string) error {
	allowed, err := selectableColumnsDB(ctx, db, module, table)
	if err != nil {
		return err
	}
	for _, c := range cols {
		if _, ok := allowed[c]; !ok {
			return fmt.Errorf("column %q is not selectable for %s/%s", c, module, table)
		}
	}
	return nil
}
