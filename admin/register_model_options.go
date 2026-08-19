package admin

import (
	"github.com/sjlit/aeus/admin/models"
)

// RegisterModelOption mutates the per-call behavior of RegisterModel.
//
//   - WithRegisterMenuSpec overrides the MenuProvider's MenuEntry()
//     result for this single model — useful when an application model
//     does NOT implement MenuProvider itself but the operator still
//     wants a matching sys_menus row, OR when an existing
//     MenuProvider.MenuEntry() needs to be tweaked at registration
//     site (icon, sort, parent, ...) without forking the model type.
//
//   - WithRegisterScenarios overrides the scenario set used to seed
//     sys_permissions rows.  Defaults to whatever permissionScenarios
//     resolves (the model's ScenarioProvider, or the canonical 6
//     scenarios).  Per-call override wins whenever it is non-nil —
//     including an empty slice, which suppresses permission seeding
//     for the model.
//
//   - WithRegisterVueOutputDir opts this single model into Vue
//     generation and (optionally) overrides the directory inherited
//     from Server.Options.VueOutputDir.  Passing "" with no
//     server-level VueOutputDir enabled keeps generation off for
//     this model; otherwise the per-call path wins over the
//     server-level default.
//
// All three are independently opt-in.  RegisterModel itself remains
// the same model-driven default; the per-call option is the
// composition point for callers that need to override one slice of
// the behavior without rewriting their model types.
//
// Backward-compatible: RegisterModel keeps its (model) signature;
// passing any RegisterModelOption is purely additive.
type RegisterModelOption func(*registerModelConfig)

type registerModelConfig struct {
	// menuSpec, when non-nil, replaces MenuProvider.MenuEntry().
	// Non-nil semantics matter: a zero MenuSpec is a valid signal
	// ("no menu row") and must NOT be confused with the default
	// "follow MenuProvider".  Use a pointer for that distinction.
	menuSpec *models.MenuSpec

	// scenarios, when non-nil, replaces permissionScenarios. As
	// above, non-nil means "explicit"; nil means "follow the model".
	// Use a non-nil empty slice to suppress permission seeding.
	scenarios []string

	// vueOutputDir, when non-nil, opts this model into Vue
	// generation (independent of the server-level VueOutputDir
	// presence).  Non-nil semantics again: the per-call value is
	// always honored when set; the server-level default is only
	// used when this stays nil.
	vueOutputDir *string
}

// WithRegisterMenuSpec overrides MenuProvider.MenuEntry() for the
// model being registered. The Component / Uri / ViewPath fields are
// still derived from ModuleName + TableName when left blank in the
// supplied spec — matching fillDerivedSpec's empty-means-derive
// contract so an override doesn't have to repeat the boilerplate.
//
// Pass an empty MenuSpec{} (not a nil *MenuSpec) to suppress the
// auto-created menu row entirely for this model while still letting
// the rest of RegisterModel run (permission seeding, AutoMigrate,
// etc.).
func WithRegisterMenuSpec(spec models.MenuSpec) RegisterModelOption {
	return func(c *registerModelConfig) {
		c.menuSpec = &spec
	}
}

// WithRegisterScenarios overrides the scenario set used to seed
// sys_permissions rows. Pass the exact list you want — append-only
// shorthand is intentionally avoided so the caller can also remove
// default-on scenarios (e.g. drop the `export` row for an immutable
// audit model).
//
// Passing zero scenarios suppresses permission seeding for the
// model entirely. Useful for models whose security posture is
// handled outside the API catalog (e.g. models whose only writes
// happen through admin tooling with its own permission scope).
func WithRegisterScenarios(scenarios ...string) RegisterModelOption {
	return func(c *registerModelConfig) {
		// Copy to a fresh slice so callers can't mutate the
		// backing array via their own helpers later.
		out := make([]string, len(scenarios))
		copy(out, scenarios)
		c.scenarios = out
	}
}

// WithRegisterVueOutputDir opts this single model into Vue
// generation. The value is the base directory (the conventional
// `views/` directory in Vite projects); the model name and singular
// are appended by the generator itself, mirroring deriveViewPath.
//
// Passing an empty string explicitly disables Vue generation for
// this model regardless of the server-level VueOutputDir — useful
// for marking one model as a hand-written view that the generator
// must never touch (it would otherwise happily overwrite a developer's
// customised Index.vue with the canonical template on the next
// restart, then silently exit on the second restart — a confusing
// two-step regression).  To inherit the server-level default, simply
// omit the option.
//
// The *string distinguishes "not configured" (nil; inherit) from
// "explicitly empty" (&"", disable) from "explicitly non-empty"
// (redirect to a different directory than the server default).
func WithRegisterVueOutputDir(dir string) RegisterModelOption {
	return func(c *registerModelConfig) {
		c.vueOutputDir = &dir
	}
}

// resolveVueOutputDir picks the directory used to generate Vue
// files for one model. Per-call setting wins; absent that, the
// server-level default is used; absent both, generation is off.
//
// Returns "" when Vue generation is disabled — callers should
// short-circuit on empty rather than treating it as "the current
// directory".
func (c *registerModelConfig) resolveVueOutputDir(serverDefault string) string {
	if c == nil {
		return serverDefault
	}
	if c.vueOutputDir != nil {
		return *c.vueOutputDir
	}
	return serverDefault
}
