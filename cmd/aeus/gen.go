package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type resourceManifest struct {
	Source        string            `json:"source"`
	GoPackage     string            `json:"go_package"`
	GoPackageName string            `json:"go_package_name"`
	Module        *manifestModule   `json:"module"`
	Messages      []manifestMessage `json:"messages"`
	Enums         []manifestEnum    `json:"enums"`
	Entities      []manifestEntity  `json:"entities"`
	Services      []manifestService `json:"services"`
}

type manifestEnum struct {
	GoName string              `json:"go_name"`
	Values []manifestEnumValue `json:"values"`
}

type manifestEnumValue struct {
	Name   string `json:"name"`
	Number int32  `json:"number"`
}

type manifestMessage struct {
	GoName string          `json:"go_name"`
	Fields []manifestField `json:"fields"`
}

type manifestField struct {
	Name string       `json:"name"`
	Type string       `json:"type"`
	Ref  *manifestRef `json:"ref"`
	List bool         `json:"list"`
	Map  bool         `json:"map"`
}

type manifestRef struct {
	Kind    string `json:"kind"`
	Package string `json:"package"`
	Name    string `json:"name"`
}

type manifestModule struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	Icon      string `json:"icon"`
	Component string `json:"component"`
	Sort      int64  `json:"sort"`
	VarName   string `json:"var_name"`
}

type manifestEntity struct {
	GoName          string   `json:"go_name"`
	ModelName       string   `json:"model_name"`
	Table           string   `json:"table"`
	Module          string   `json:"module"`
	Singular        string   `json:"singular"`
	Plural          string   `json:"plural"`
	ViewPath        string   `json:"view_path"`
	MenuName        string   `json:"menu_name"`
	Component       string   `json:"component"`
	Base            string   `json:"base"`
	Scenarios       []string `json:"scenarios"`
	PermissionGroup string   `json:"permission_group"`
	ReadOnly        bool     `json:"read_only"`
}

type manifestService struct {
	GoName      string           `json:"go_name"`
	PoliciesVar string           `json:"policies_var"`
	Policies    []manifestPolicy `json:"policies"`
	Methods     []manifestMethod `json:"methods"`
}

type manifestMethod struct {
	GoName      string       `json:"go_name"`
	Request     string       `json:"request"`
	Response    string       `json:"response"`
	RequestRef  *manifestRef `json:"request_ref"`
	ResponseRef *manifestRef `json:"response_ref"`
	HTTPMethod  string       `json:"http_method"`
	HTTPPath    string       `json:"http_path"`
	Body        string       `json:"body"`
}

type manifestPolicy struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Button      string `json:"button"`
	Group       string `json:"group"`
	Description string `json:"description"`
	Public      bool   `json:"public"`
}

func runGen(args []string) error {
	flags := flag.NewFlagSet("gen", flag.ContinueOnError)
	protoDir := flags.String("proto-dir", "proto", "directory containing *_aeus.json manifests")
	outDir := flags.String("out", "internal/bootstrap", "output directory")
	pkg := flags.String("package", "bootstrap", "Go package name for the generated file")
	name := flags.String("file", "generated.go", "generated file name")
	webDir := flags.String("web-dir", "web/src/views", "Vue views directory")
	apiDir := flags.String("api-dir", "web/src/api/generated", "TypeScript API output directory")
	noWeb := flags.Bool("no-web", false, "skip Vue page generation")
	noAPI := flags.Bool("no-api", false, "skip TypeScript API generation")
	check := flags.Bool("check", false, "verify generated files are up to date")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("gen: unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	manifests, err := loadManifests(*protoDir)
	if err != nil {
		return err
	}
	source, err := renderGenerated(*pkg, manifests)
	if err != nil {
		return err
	}
	formatted, err := format.Source(source)
	if err != nil {
		return fmt.Errorf("gofmt generated source: %w\n%s", err, source)
	}

	target := filepath.Join(*outDir, *name)
	if *check {
		existing, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read %s: %w", target, err)
		}
		if !bytes.Equal(existing, formatted) {
			return fmt.Errorf("%s is out of date; run `aeus gen`", target)
		}
		fmt.Printf("%s is up to date\n", target)
		var allMissing []string
		if !*noWeb {
			_, skipped, missing, err := generateVueFiles(manifests, *webDir, true)
			if err != nil {
				return err
			}
			allMissing = append(allMissing, missing...)
			fmt.Printf("%d Vue page(s) present (%d existing)\n", skipped+len(missing), skipped)
		}
		if !*noAPI {
			_, skipped, missing, err := generateAPIFiles(manifests, *apiDir, true)
			if err != nil {
				return err
			}
			allMissing = append(allMissing, missing...)
			fmt.Printf("%d TypeScript API file(s) present (%d existing)\n", skipped+len(missing), skipped)
		}
		if len(allMissing) > 0 {
			return fmt.Errorf("missing generated file(s): %s", strings.Join(allMissing, ", "))
		}
		return nil
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, formatted, 0o644); err != nil {
		return err
	}
	fmt.Printf("generated %s from %d manifest(s)\n", target, len(manifests))
	if !*noWeb {
		written, skipped, _, err := generateVueFiles(manifests, *webDir, false)
		if err != nil {
			return err
		}
		fmt.Printf("generated %d Vue page(s), skipped %d existing\n", written, skipped)
	}
	if !*noAPI {
		written, skipped, _, err := generateAPIFiles(manifests, *apiDir, false)
		if err != nil {
			return err
		}
		fmt.Printf("generated %d TypeScript API file(s), skipped %d existing\n", written, skipped)
	}
	return nil
}

func loadManifests(root string) ([]resourceManifest, error) {
	var manifests []resourceManifest
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_aeus.json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var m resourceManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if m.GoPackage == "" {
			return fmt.Errorf("%s: go_package is empty", path)
		}
		source := filepath.Clean(filepath.FromSlash(m.Source))
		if m.Source == "" || source == "." || filepath.IsAbs(source) ||
			source == ".." || strings.HasPrefix(source, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s: invalid proto source %q", path, m.Source)
		}
		found := false
		for _, base := range []string{root, filepath.Dir(root)} {
			if _, err := os.Stat(filepath.Join(base, source)); err == nil {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: stale manifest for %q (source not found under %s); rerun `make proto` or remove stale generated files",
				path, m.Source, root)
		}
		manifests = append(manifests, m)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Source < manifests[j].Source })
	return manifests, nil
}

type genModule struct {
	alias   string
	varName string
	spec    *manifestModule
}

type genEntity struct {
	alias string
	model string
	name  string
}

type genPolicy struct {
	alias  string
	name   string
	goName string
}

func renderGenerated(pkg string, manifests []resourceManifest) ([]byte, error) {
	aliases := map[string]string{}
	usedAliases := map[string]bool{"admin": true, "gorm": true, "fmt": true}
	for _, m := range manifests {
		if _, ok := aliases[m.GoPackage]; ok {
			continue
		}
		alias := aliasName(m.GoPackageName)
		base := alias
		for n := 2; usedAliases[alias]; n++ {
			alias = fmt.Sprintf("%s_%d", base, n)
		}
		usedAliases[alias] = true
		aliases[m.GoPackage] = alias
	}

	var (
		modules  []genModule
		entities []genEntity
		policies []genPolicy
	)
	usedPackages := map[string]bool{}
	seenModule := map[string]bool{}
	seenEntity := map[string]bool{}
	seenPolicy := map[string]bool{}
	for _, m := range manifests {
		alias := aliases[m.GoPackage]
		if m.Module != nil {
			key := m.Module.Component
			if key == "" {
				key = m.Module.Name
			}
			if !seenModule[key] {
				seenModule[key] = true
				varName := m.Module.VarName
				if varName == "" {
					varName = "Module"
				}
				modules = append(modules, genModule{alias: alias, varName: varName, spec: m.Module})
				usedPackages[m.GoPackage] = true
			}
		}
		for _, e := range m.Entities {
			key := m.GoPackage + "." + e.ModelName
			if seenEntity[key] {
				continue
			}
			seenEntity[key] = true
			entities = append(entities, genEntity{alias: alias, model: e.ModelName, name: e.GoName})
			usedPackages[m.GoPackage] = true
		}
		for _, s := range m.Services {
			if s.PoliciesVar == "" {
				continue
			}
			key := m.GoPackage + "." + s.PoliciesVar
			if seenPolicy[key] {
				continue
			}
			seenPolicy[key] = true
			policies = append(policies, genPolicy{alias: alias, name: s.PoliciesVar, goName: s.GoName})
			usedPackages[m.GoPackage] = true
		}
	}
	sort.Slice(modules, func(i, j int) bool { return moduleKey(modules[i].spec) < moduleKey(modules[j].spec) })
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].alias+"."+entities[i].model < entities[j].alias+"."+entities[j].model
	})
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].alias+"."+policies[i].name < policies[j].alias+"."+policies[j].name
	})

	hasOps := len(modules)+len(entities)+len(policies) > 0
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by aeus gen. DO NOT EDIT.\n\npackage %s\n\n", pkg)
	fmt.Fprintln(&b, "import (")
	if hasOps {
		fmt.Fprintln(&b, "\t\"fmt\"")
	}
	fmt.Fprintln(&b, "\t\"github.com/sjlit/aeus/admin\"")
	fmt.Fprintln(&b, "\t\"gorm.io/gorm\"")
	paths := make([]string, 0, len(usedPackages))
	for path := range usedPackages {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Fprintf(&b, "\t%s %q\n", aliases[path], path)
	}
	fmt.Fprintln(&b, ")")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "// RegisterGenerated wires proto-generated business models, module menus")
	fmt.Fprintln(&b, "// and route permissions into the admin server. It is called after Setup.")
	fmt.Fprintln(&b, "func RegisterGenerated(s *admin.Server, db *gorm.DB) error {")
	for _, m := range modules {
		fmt.Fprintf(&b, "\tif err := admin.EnsureModuleMenu(db, %s.%s); err != nil {\n", m.alias, m.varName)
		fmt.Fprintf(&b, "\t\treturn fmt.Errorf(%q, err)\n", "ensure module "+m.spec.Name+": %w")
		fmt.Fprintln(&b, "\t}")
	}
	for _, e := range entities {
		fmt.Fprintf(&b, "\tif err := s.RegisterModel(&%s.%s{}); err != nil {\n", e.alias, e.model)
		fmt.Fprintf(&b, "\t\treturn fmt.Errorf(%q, err)\n", "register "+e.name+": %w")
		fmt.Fprintln(&b, "\t}")
	}
	for _, p := range policies {
		fmt.Fprintf(&b, "\tif err := admin.EnsureEndpointPermissions(db, %s.%s...); err != nil {\n", p.alias, p.name)
		fmt.Fprintf(&b, "\t\treturn fmt.Errorf(%q, err)\n", "register policies for "+p.goName+": %w")
		fmt.Fprintln(&b, "\t}")
	}
	fmt.Fprintln(&b, "\treturn nil")
	fmt.Fprintln(&b, "}")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "// MigrateGenerated creates the tables for proto-generated entities.")
	fmt.Fprintln(&b, "func MigrateGenerated(db *gorm.DB) error {")
	if len(entities) == 0 {
		fmt.Fprintln(&b, "\treturn nil")
	} else {
		fmt.Fprint(&b, "\treturn db.AutoMigrate(")
		for i, e := range entities {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "&%s.%s{}", e.alias, e.model)
		}
		fmt.Fprintln(&b, ")")
	}
	fmt.Fprintln(&b, "}")
	return b.Bytes(), nil
}

func moduleKey(m *manifestModule) string {
	if m == nil {
		return ""
	}
	if m.Component != "" {
		return m.Component
	}
	return m.Name
}

func aliasName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "pkg"
	}
	var b strings.Builder
	for i, r := range s {
		if unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "pkg"
	}
	return b.String()
}
