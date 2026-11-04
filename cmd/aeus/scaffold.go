package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

//go:embed all:templates
var templateFS embed.FS

//go:embed versions.json
var versionsJSON []byte

// ignoredTemplateDirs keeps local build artifacts out of generated
// projects. They are git-ignored, but `go run ./cmd/aeus` embeds the
// working tree, so a developer who ran npm install in the template would
// otherwise copy node_modules into every generated project.
var ignoredTemplateDirs = map[string]bool{
	"node_modules": true,
	"dist":         true,
	".git":         true,
	".mimocode":    true,
}

func loadVersions() (map[string]string, error) {
	var versions map[string]string
	if err := json.Unmarshal(versionsJSON, &versions); err != nil {
		return nil, fmt.Errorf("parse embedded versions.json: %w", err)
	}
	for _, key := range []string{"root", "transport/http", "admin"} {
		if versions[key] == "" {
			return nil, fmt.Errorf("versions.json is missing %q", key)
		}
	}
	return versions, nil
}

func renderTemplate(templateName, target string, replacements map[string]string) error {
	root, err := fs.Sub(templateFS, filepath.ToSlash(filepath.Join("templates", templateName)))
	if err != nil {
		return err
	}
	pairs := make([]string, 0, len(replacements)*2)
	for old, newValue := range replacements {
		pairs = append(pairs, old, newValue)
	}
	replacer := strings.NewReplacer(pairs...)

	return fs.WalkDir(root, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		// .tmpl is stripped for files that must not be interpreted as
		// part of this module by the Go toolchain (notably go.mod).
		destRel := strings.TrimSuffix(path, ".tmpl")
		dest := filepath.Join(target, filepath.FromSlash(destRel))
		if entry.IsDir() {
			if ignoredTemplateDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return os.MkdirAll(dest, 0o755)
		}
		data, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		data = []byte(replacer.Replace(string(data)))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}

func gofmtTree(target string) error {
	return filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		formatted, err := format.Source(src)
		if err != nil {
			return fmt.Errorf("gofmt %s: %w", path, err)
		}
		return os.WriteFile(path, formatted, 0o644)
	})
}

// tidyProject runs `go mod tidy` so the generated project is immediately
// buildable. A failure is non-fatal: the user may be offline or the
// published AEUS version may not exist yet (use --local during the
// pre-release window).
func tidyProject(target string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = target
	// A parent go.work would otherwise pull the generated project into
	// the surrounding workspace and hide missing go.mod entries.
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: go mod tidy failed (run it manually): %v\n%s\n", err, output)
	}
}

func replaceDirectives(localPath, templateName string) (string, error) {
	if localPath == "" {
		return "", nil
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return "", err
	}
	check := func(modulePath string) error {
		path := filepath.Join(abs, filepath.FromSlash(modulePath))
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err != nil {
			return fmt.Errorf("local AEUS checkout %s is missing %s/go.mod: %w", abs, modulePath, err)
		}
		return nil
	}
	if err := check("."); err != nil {
		return "", err
	}
	if err := check("transport/http"); err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("replace github.com/sjlit/aeus => %q", abs),
		fmt.Sprintf("replace github.com/sjlit/aeus/transport/http => %q", filepath.Join(abs, "transport", "http")),
	}
	if templateName == "admin" {
		if err := check("admin"); err != nil {
			return "", err
		}
		lines = append(lines,
			fmt.Sprintf("replace github.com/sjlit/aeus/admin => %q", filepath.Join(abs, "admin")),
		)
	}
	return strings.Join(lines, "\n"), nil
}

// detectLocalCheckout makes `go run ./cmd/aeus new ...` work without
// --local while the coordinated release is still pending. Installed
// binaries report a version tag and therefore use versions.json.
func detectLocalCheckout() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Path != "github.com/sjlit/aeus" || info.Main.Version != "(devel)" {
		return ""
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.HasPrefix(strings.TrimSpace(string(data)), "module github.com/sjlit/aeus") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == '_', r == '.', r == ' ', r == '\t':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func packageName(s string) string {
	s = sanitizeName(s)
	if s == "" {
		return "app"
	}
	return s
}

func validateModulePath(module string) error {
	if module == "" {
		return fmt.Errorf("module path is empty")
	}
	if strings.ContainsAny(module, " \t\r\n@\\:") {
		return fmt.Errorf("module path %q contains an invalid character", module)
	}
	if strings.HasPrefix(module, "/") || strings.HasSuffix(module, "/") || strings.Contains(module, "//") {
		return fmt.Errorf("module path %q has an empty path element", module)
	}
	for _, element := range strings.Split(module, "/") {
		if element == "" || element == "." || element == ".." {
			return fmt.Errorf("module path %q has an invalid path element", module)
		}
	}
	return nil
}
