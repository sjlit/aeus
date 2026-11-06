// Command syncweb regenerates cmd/aeus/templates/admin/web from
// admin/web.
//
// The generated template keeps a small set of files that are specific to
// the scaffold (README, .gitignore, the smoke test and the checked-in
// components.d.ts); everything else is copied verbatim, with a handful of
// placeholder substitutions applied.
//
// Run it via:
//
//	go generate ./cmd/aeus
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// preserved files are owned by the template and never copied from admin/web.
var preserved = []string{
	"README.md",
	".gitignore",
	"src/__tests__/smoke.test.ts",
	"src/components.d.ts",
}

var skippedDirs = map[string]bool{
	"node_modules": true,
	"dist":         true,
	".mimocode":    true,
	".git":         true,
	"__tests__":    true,
}

func main() {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fail("cannot locate source file")
	}
	// .../cmd/aeus/internal/syncweb/main.go -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	src := filepath.Join(root, "admin", "web")
	dst := filepath.Join(root, "cmd", "aeus", "templates", "admin", "web")

	if err := syncDir(src, dst); err != nil {
		fail(err.Error())
	}
	fmt.Printf("synced %s -> %s\n", filepath.ToSlash(src), filepath.ToSlash(dst))
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "syncweb:", msg)
	os.Exit(1)
}

// syncDir rebuilds dst from src while preserving template-owned files.
func syncDir(src, dst string) error {
	keep, err := readPreserved(src, dst)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("remove %s: %w", dst, err)
	}

	copied := 0
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := keep[rel]; ok {
			return nil
		}
		if strings.Contains(rel, "__tests__/") {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body = transform(rel, body)
		if err := writeFile(dst, rel, body); err != nil {
			return err
		}
		copied++
		return nil
	})
	if err != nil {
		return fmt.Errorf("copy admin/web: %w", err)
	}

	for rel, body := range keep {
		if err := writeFile(dst, rel, body); err != nil {
			return err
		}
		copied++
	}
	fmt.Printf("copied %d files (%d preserved from template)\n", copied, len(keep))
	return nil
}

// readPreserved keeps the template-owned files, and refreshes
// components.d.ts from admin/web when it exists locally (it is generated
// and git-ignored there, so a clean checkout will not have it).
func readPreserved(src, dst string) (map[string][]byte, error) {
	keep := make(map[string][]byte, len(preserved))
	for _, rel := range preserved {
		body, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err == nil {
			keep[rel] = body
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read preserved %s: %w", rel, err)
		}
	}
	if body, err := os.ReadFile(filepath.Join(src, "src", "components.d.ts")); err == nil {
		keep["src/components.d.ts"] = body
	}
	return keep, nil
}

func writeFile(root, rel string, body []byte) error {
	out := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, body, 0o644)
}

// transform applies the scaffold-specific substitutions. The replacements
// are intentionally literal so the generated files keep their original
// formatting (JSON re-encoding would reorder package-lock.json).
func transform(rel string, body []byte) []byte {
	switch rel {
	case ".env.production":
		// The generated project is served by the Go binary, so the API is
		// same-origin. admin/web uses /api for its separate dev setup.
		return []byte("VITE_API_BASE=\n")
	case "index.html":
		body = bytes.ReplaceAll(body,
			[]byte("<title>Board</title>"),
			[]byte("<title>__AEUS_NAME__</title>"))
		return bytes.ReplaceAll(body,
			[]byte(`<div class="boot">Board</div>`),
			[]byte(`<div class="boot">__AEUS_NAME__</div>`))
	case "public/app-config.js":
		// admin/web keeps its own default brand; generated projects start
		// with the project name. appMark is omitted so brand.ts derives it
		// from the first character of appName.
		return []byte("window.__AEUS_CONFIG__ = {\n  appName: '__AEUS_NAME__',\n}\n")
	case "package.json", "package-lock.json":
		return bytes.ReplaceAll(body,
			[]byte(`"name": "aeus"`),
			[]byte(`"name": "__AEUS_PACKAGE__"`))
	default:
		return body
	}
}
