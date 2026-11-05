// Command aeus scaffolds a new AEUS project.
//
// Usage:
//
//	aeus new <name> [--template=minimal|admin] [--module=path] [--local=/path/to/aeus] [--force]
//
//go:generate go run ./internal/syncweb
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "new":
		if err := runNew(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "aeus:", err)
			os.Exit(1)
		}
	case "gen":
		if err := runGen(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "aeus:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "aeus: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage:
  aeus new <name> [flags]
  aeus gen [flags]

Flags for new:
  --template string   minimal (default) or admin
  --module string     Go module path (default: the project name)
  --local string      path to an AEUS checkout; emits replace directives
  --force             write into a non-empty directory

Flags for gen:
  --proto-dir string   directory containing *_aeus.json (default "proto")
  --out string         output directory (default "internal/gen")
  --package string     Go package name (default "gen")
  --file string        generated file name (default "wiring.go")
  --web-dir string     Vue views directory (default "web/src/views")
  --api-dir string     TypeScript API directory (default "web/src/api/generated")
  --no-web             skip Vue page generation
  --no-api             skip TypeScript API generation
  --check              verify generated Go/Vue/API outputs (TS content, Vue presence)

Examples:
  aeus new order-svc
  aeus new order-svc --template=admin
  aeus new order-svc --module github.com/acme/order-svc
  aeus new order-svc --local=/workspace/go/aeus
  aeus gen
  aeus gen --check
`)
}

func runNew(args []string) error {
	templateFlag := "minimal"
	moduleFlag := ""
	localFlag := ""
	force := false
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func(flagName string) (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", flagName)
			}
			i++
			return args[i], nil
		}
		switch {
		case arg == "-h" || arg == "--help":
			usage()
			return nil
		case arg == "--template" || arg == "-template":
			v, err := next(arg)
			if err != nil {
				return err
			}
			templateFlag = v
		case strings.HasPrefix(arg, "--template="):
			templateFlag = strings.TrimPrefix(arg, "--template=")
		case arg == "--module" || arg == "-module":
			v, err := next(arg)
			if err != nil {
				return err
			}
			moduleFlag = v
		case strings.HasPrefix(arg, "--module="):
			moduleFlag = strings.TrimPrefix(arg, "--module=")
		case arg == "--local" || arg == "-local":
			v, err := next(arg)
			if err != nil {
				return err
			}
			localFlag = v
		case strings.HasPrefix(arg, "--local="):
			localFlag = strings.TrimPrefix(arg, "--local=")
		case arg == "--force" || arg == "-force":
			force = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q", arg)
		default:
			if target != "" {
				return fmt.Errorf("expected exactly one project name")
			}
			target = arg
		}
	}
	if target == "" {
		return fmt.Errorf("expected exactly one project name")
	}
	name := sanitizeName(baseName(target))
	if name == "" {
		return fmt.Errorf("project name must contain at least one letter or digit")
	}
	module := moduleFlag
	if module == "" {
		module = name
	}
	if err := validateModulePath(module); err != nil {
		return err
	}
	if templateFlag != "minimal" && templateFlag != "admin" {
		return fmt.Errorf("unknown template %q (want minimal or admin)", templateFlag)
	}

	if !force {
		entries, err := os.ReadDir(target)
		if err == nil && len(entries) > 0 {
			return fmt.Errorf("%s is not empty; pass --force to overwrite template files", target)
		}
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}

	versions, err := loadVersions()
	if err != nil {
		return err
	}
	local := localFlag
	if local == "" {
		local = detectLocalCheckout()
		if local != "" {
			fmt.Printf("Using local AEUS checkout: %s\n", local)
		}
	}
	replaces, err := replaceDirectives(local, templateFlag)
	if err != nil {
		return err
	}
	replacements := map[string]string{
		"__AEUS_NAME__":          name,
		"__AEUS_MODULE__":        module,
		"__AEUS_PACKAGE__":       packageName(name),
		"__AEUS_ROOT_VERSION__":  versions["root"],
		"__AEUS_HTTP_VERSION__":  versions["transport/http"],
		"__AEUS_ADMIN_VERSION__": versions["admin"],
		"__AEUS_REPLACES__":      replaces,
	}
	if err := renderTemplate(templateFlag, target, replacements); err != nil {
		return err
	}
	if err := gofmtTree(target); err != nil {
		return err
	}
	tidyProject(target)

	fmt.Printf("Created %s (template: %s, module: %s)\n\n", target, templateFlag, module)
	if templateFlag == "admin" {
		fmt.Printf("Next:\n  cd %s\n  make web\n  make run\n\n", target)
		fmt.Printf("Open http://localhost:8080 and sign in with %s / Admin123.\n", "admin")
		fmt.Printf("Change the default password and JWT_SECRET before exposing it.\n")
	} else {
		fmt.Printf("Next:\n  cd %s\n  make run\n\n", target)
		fmt.Printf("Health: http://localhost:8080/health\n")
	}
	return nil
}

func baseName(p string) string {
	for len(p) > 1 && (p[len(p)-1] == '/' || p[len(p)-1] == '\\') {
		p = p[:len(p)-1]
	}
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
