package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncDir(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "admin", "web")
	dst := filepath.Join(root, "cmd", "aeus", "templates", "admin", "web")

	writeTestFile(t, src, "package.json", `{"name": "aeus"}`)
	writeTestFile(t, src, "package-lock.json", "{\n  \"name\": \"aeus\",\n  \"packages\": {\n    \"\": {\n      \"name\": \"aeus\"\n    }\n  }\n}\n")
	writeTestFile(t, src, "index.html", "<title>aeus</title>")
	writeTestFile(t, src, ".env.production", "VITE_API_BASE=/api\n")
	writeTestFile(t, src, "src/main.ts", "export {}\n")
	writeTestFile(t, src, "src/components.d.ts", "// generated from admin/web\n")
	writeTestFile(t, src, "src/api/__tests__/envelope.test.ts", "// admin test\n")
	writeTestFile(t, src, "node_modules/pkg/index.js", "// dependency\n")
	writeTestFile(t, src, "dist/app.js", "// build output\n")
	writeTestFile(t, src, ".mimocode/config", "// editor config\n")

	writeTestFile(t, dst, "README.md", "# template readme\n")
	writeTestFile(t, dst, ".gitignore", "node_modules/\n")
	writeTestFile(t, dst, "src/__tests__/smoke.test.ts", "// smoke\n")
	writeTestFile(t, dst, "src/components.d.ts", "// template fallback\n")
	writeTestFile(t, dst, "stale.txt", "remove me\n")

	if err := syncDir(src, dst); err != nil {
		t.Fatalf("syncDir: %v", err)
	}

	assertFile(t, dst, "README.md", "# template readme\n")
	assertFile(t, dst, ".gitignore", "node_modules/\n")
	assertFile(t, dst, "src/__tests__/smoke.test.ts", "// smoke\n")
	assertFile(t, dst, "src/components.d.ts", "// generated from admin/web\n")
	assertFile(t, dst, "package.json", `{"name": "__AEUS_PACKAGE__"}`)
	assertFile(t, dst, "index.html", "<title>__AEUS_NAME__</title>")
	assertFile(t, dst, ".env.production", "VITE_API_BASE=\n")
	assertFile(t, dst, "src/main.ts", "export {}\n")
	assertMissing(t, dst, "src/api/__tests__/envelope.test.ts")
	assertMissing(t, dst, "node_modules/pkg/index.js")
	assertMissing(t, dst, "dist/app.js")
	assertMissing(t, dst, ".mimocode/config")
	assertMissing(t, dst, "stale.txt")

	lock, err := os.ReadFile(filepath.Join(dst, "package-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(lock), `"name": "__AEUS_PACKAGE__"`); got != 2 {
		t.Fatalf("package-lock placeholder count = %d, want 2\n%s", got, lock)
	}
	if strings.Contains(string(lock), `"name": "aeus"`) {
		t.Fatalf("package-lock still contains admin name:\n%s", lock)
	}
}

func TestSyncDirPreservesComponentsWhenSourceMissing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "admin", "web")
	dst := filepath.Join(root, "cmd", "aeus", "templates", "admin", "web")

	writeTestFile(t, src, "src/main.ts", "export {}\n")
	writeTestFile(t, dst, "README.md", "# template readme\n")
	writeTestFile(t, dst, "src/__tests__/smoke.test.ts", "// smoke\n")
	writeTestFile(t, dst, "src/components.d.ts", "// template fallback\n")

	if err := syncDir(src, dst); err != nil {
		t.Fatalf("syncDir: %v", err)
	}
	assertFile(t, dst, "src/components.d.ts", "// template fallback\n")
}

func writeTestFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, root, rel, want string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if string(body) != want {
		t.Fatalf("%s = %q, want %q", rel, body, want)
	}
}

func assertMissing(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist (err=%v)", rel, err)
	}
}
