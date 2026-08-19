package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVuePathForModel pins the path-composition rule used for the
// generated file. The directory layout must mirror deriveViewPath so
// that the on-disk address and the sys_menus.view_path stored in the
// database line up exactly — otherwise the front-end's glob loader
// would resolve the view_path to a file that doesn't exist (or worse,
// a wrong file).
func TestVuePathForModel(t *testing.T) {
	cases := []struct {
		name     string
		dir      string
		module   string
		singular string
		want     string
	}{
		{
			// sys_users -> singular "sys_user", module stays lowercase.
			name:     "module lowercased, singular preserved",
			dir:      "/abs/views",
			module:   "system",
			singular: "sys_user",
			want:     filepath.Join("/abs/views", "system", "sys_user", "Index.vue"),
		},
		{
			// Empty module -> empty path (same skip signal as
			// deriveViewPath).
			name:     "empty module returns empty",
			dir:      "/abs/views",
			module:   "",
			singular: "sys_user",
			want:     "",
		},
		{
			name:     "empty singular returns empty",
			dir:      "/abs/views",
			module:   "system",
			singular: "",
			want:     "",
		},
		{
			// Already-lowercase module: stays unchanged.
			name:     "module stays lowercase (no dash conversion)",
			dir:      "/abs/views",
			module:   "workspace",
			singular: "ticket",
			want:     filepath.Join("/abs/views", "workspace", "ticket", "Index.vue"),
		},
		{
			// Path-traversal guard: a hostile TableName returning
			// "../../etc" would otherwise resolve to /etc/passwd/Index.vue.
			// vuePathForModel must refuse it. See the docstring on
			// vuePathForModel for why the guard is in place.
			name:     "module with traversal rejected",
			dir:      "/abs/views",
			module:   "..",
			singular: "passwd",
			want:     "",
		},
		{
			name:     "singular with traversal rejected",
			dir:      "/abs/views",
			module:   "system",
			singular: "../etc",
			want:     "",
		},
		{
			name:     "module with path separator rejected",
			dir:      "/abs/views",
			module:   "system/extra",
			singular: "sys_user",
			want:     "",
		},
		{
			name:     "singular with backslash rejected",
			dir:      "/abs/views",
			module:   "system",
			singular: `sys_user\bad`,
			want:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := vuePathForModel(tc.dir, tc.module, tc.singular); got != tc.want {
				t.Fatalf("vuePathForModel(%q,%q,%q):\n  got:  %q\n  want: %q",
					tc.dir, tc.module, tc.singular, got, tc.want)
			}
		})
	}
}

// TestGenerateVueFile_SkipExisting is the idempotency contract: a
// file that already exists on disk must NOT be overwritten.  The
// pre-created payload has a unique sentinel; the test fails loudly if
// the run touches the file.
func TestGenerateVueFile_SkipExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "Index.vue")
	// Pre-populate with sentinel content. MkdirAll on the parent is
	// the test's responsibility because generateVueFile's MkdirAll
	// only runs when the write proceeds.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const sentinel = "/* sentinel — keep me */"
	if err := os.WriteFile(path, []byte(sentinel), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	written, err := generateVueFile(path, "// should never land\n")
	if err != nil {
		t.Fatalf("generateVueFile: %v", err)
	}
	if written {
		t.Fatalf("generateVueFile reported written=true on existing file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != sentinel {
		t.Fatalf("existing file was overwritten:\n  got:  %q\n  want: %q", got, sentinel)
	}
}

// TestGenerateVueFile_WritesWhenMissing exercises the happy path:
// missing file -> MkdirAll creates the parents and the body lands.
// The body is whatever the caller passes — the test passes a known
// string so the assertion can pin it byte-for-byte.
func TestGenerateVueFile_WritesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "system", "sys_user", "Index.vue")
	const body = "<template>hello</template>\n"

	written, err := generateVueFile(path, body)
	if err != nil {
		t.Fatalf("generateVueFile: %v", err)
	}
	if !written {
		t.Fatalf("generateVueFile reported written=false on missing file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != body {
		t.Fatalf("body mismatch:\n  got:  %q\n  want: %q", got, body)
	}
}

// TestBuildVueContent_Placeholders pins the four placeholder
// substitutions against a fake resource. The component name follows
// the project's existing convention (PascalCase module +
// PascalCase pluralized table); sys_users is a convenient example
// that already exists in admin/web/src/views/.
func TestBuildVueContent_Placeholders(t *testing.T) {
	// Rest/v3 requires an actual resource handle to call
	// resource.ModelValue().GetNaming(); the simpler path is to
	// pin the substitution strings directly by re-deriving them
	// with the public helpers. This keeps the test independent of
	// rest/v3's internals — if it ever refactors how Naming is
	// resolved, this test stays focused on the template itself.
	module := "system"
	table := "sys_users"
	singular := "sys_user"
	pluralize := "sys_users"

	body := vueTemplate
	body = strings.ReplaceAll(body, "{{MODULE}}", strings.ToLower(module))
	body = strings.ReplaceAll(body, "{{TABLE}}", table)
	body = strings.ReplaceAll(body, "{{SINGULAR}}", singular)
	body = strings.ReplaceAll(body, "{{COMPONENT_NAME}}", pascalWords(module)+pascalWords(pluralize))

	// All placeholders must have been substituted.
	for _, ph := range []string{"{{MODULE}}", "{{TABLE}}", "{{SINGULAR}}", "{{COMPONENT_NAME}}"} {
		if strings.Contains(body, ph) {
			t.Fatalf("template still contains placeholder %s after substitution:\n%s", ph, body)
		}
	}
	// And the canonical sys_users component name (matching the
	// hand-written admin/web/src/views/system/sys_user/Index.vue)
	// must appear exactly once in defineOptions.
	if !strings.Contains(body, "defineOptions({ name: 'SystemSysUsers' })") {
		t.Fatalf("expected canonical component name in body, got:\n%s", body)
	}
	// And the SchemaViewer must target module=system / table=sys_users.
	if !strings.Contains(body, `module="system" table="sys_users"`) {
		t.Fatalf("expected SchemaViewer module/table in body, got:\n%s", body)
	}
	// Title fallback uses the literal table name.
	if !strings.Contains(body, `'sys_users'`) {
		t.Fatalf("expected table-name title fallback in body, got:\n%s", body)
	}
}

// TestRegisterModelConfig_ResolveVueOutputDir pins the precedence
// rule for the per-call vs server-level VueOutputDir.
//
//   - per-call override (incl. "") wins;
//   - absent that, the server-level default is used;
//   - absent both, generation is off (returned as empty).
//
// Each entry pairs a struct value with the server default it
// should be resolved against, so the table can express the full
// cartesian product (per-call-set vs server-set) without resorting
// to a follow-up t.Run for the no-server-default case.
func TestRegisterModelConfig_ResolveVueOutputDir(t *testing.T) {
	cases := []struct {
		name           string
		cfg            *registerModelConfig
		serverDefault  string
		want           string
	}{
		{
			name:          "nil cfg falls back to server default",
			cfg:           nil,
			serverDefault: "/abs/views",
			want:          "/abs/views",
		},
		{
			name:          "per-call non-empty wins over server default",
			cfg:           &registerModelConfig{vueOutputDir: stringPtr("/other/views")},
			serverDefault: "/abs/views",
			want:          "/other/views",
		},
		{
			name:          "per-call empty explicitly disables regardless of server",
			cfg:           &registerModelConfig{vueOutputDir: stringPtr("")},
			serverDefault: "/abs/views",
			want:          "",
		},
		{
			name:          "per-call nil falls back to server default",
			cfg:           &registerModelConfig{vueOutputDir: nil},
			serverDefault: "/abs/views",
			want:          "/abs/views",
		},
		{
			name:          "no server default and no per-call -> off",
			cfg:           &registerModelConfig{vueOutputDir: nil},
			serverDefault: "",
			want:          "",
		},
		{
			name:          "per-call non-empty with empty server -> per-call",
			cfg:           &registerModelConfig{vueOutputDir: stringPtr("/other/views")},
			serverDefault: "",
			want:          "/other/views",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.resolveVueOutputDir(tc.serverDefault); got != tc.want {
				t.Fatalf("resolveVueOutputDir: got %q want %q", got, tc.want)
			}
		})
	}
}

// stringPtr is a small helper kept here (not exported) so the
// precedence test above stays compact.
func stringPtr(s string) *string { return &s }
