package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"Order_Service": "order-service",
		"My Service":    "my-service",
		"  aeus.app  ":  "aeus-app",
		"***":           "",
	}
	for input, want := range cases {
		if got := sanitizeName(input); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPackageName(t *testing.T) {
	if got := packageName("Order_Service"); got != "order-service" {
		t.Fatalf("packageName = %q, want order-service", got)
	}
}

func TestValidateModulePath(t *testing.T) {
	valid := []string{"order-svc", "github.com/acme/order-svc", "example.com/a.b/c"}
	for _, module := range valid {
		if err := validateModulePath(module); err != nil {
			t.Errorf("validateModulePath(%q): %v", module, err)
		}
	}
	invalid := []string{"", "a b", "a//b", "/a", "a/", "a/../b", "a@b", `a\b`}
	for _, module := range invalid {
		if err := validateModulePath(module); err == nil {
			t.Errorf("validateModulePath(%q) unexpectedly passed", module)
		}
	}
}

func TestRenderTemplateReplacesPlaceholders(t *testing.T) {
	target := t.TempDir()
	err := renderTemplate("minimal", target, map[string]string{
		"__AEUS_NAME__":          "demo",
		"__AEUS_MODULE__":        "github.com/acme/demo",
		"__AEUS_PACKAGE__":       "demo",
		"__AEUS_ROOT_VERSION__":  "v1.0.0",
		"__AEUS_HTTP_VERSION__":  "v1.0.0",
		"__AEUS_ADMIN_VERSION__": "v1.0.0",
		"__AEUS_REPLACES__":      "",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	goMod, err := os.ReadFile(filepath.Join(target, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goMod), "module github.com/acme/demo") {
		t.Fatalf("go.mod did not get the module path:\n%s", goMod)
	}
	main, err := os.ReadFile(filepath.Join(target, "cmd", "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(main), "__AEUS_") {
		t.Fatalf("placeholder left in main.go:\n%s", main)
	}
	if _, err := os.Stat(filepath.Join(target, "configs", "config.example.yaml")); err != nil {
		t.Fatalf("config example missing: %v", err)
	}
	err = filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "__AEUS_") {
			t.Errorf("unreplaced placeholder in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated project: %v", err)
	}
}
