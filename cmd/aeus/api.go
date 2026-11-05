package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var pathParamRE = regexp.MustCompile(`:([A-Za-z_][A-Za-z0-9_]*)`)

type apiTypeInfo struct {
	source   string
	filePath string
	name     string
}

type apiIndex struct {
	messages map[string]apiTypeInfo
	enums    map[string]apiTypeInfo
}

func buildAPIIndex(manifests []resourceManifest, apiDir string) (*apiIndex, error) {
	idx := &apiIndex{
		messages: map[string]apiTypeInfo{},
		enums:    map[string]apiTypeInfo{},
	}
	for _, m := range manifests {
		path, err := apiFilePath(apiDir, m.Source)
		if err != nil {
			return nil, err
		}
		for _, msg := range m.Messages {
			idx.messages[apiTypeKey(m.GoPackage, msg.GoName)] = apiTypeInfo{
				source: m.Source, filePath: path, name: msg.GoName,
			}
		}
		for _, e := range m.Enums {
			idx.enums[apiTypeKey(m.GoPackage, e.GoName)] = apiTypeInfo{
				source: m.Source, filePath: path, name: e.GoName,
			}
		}
	}
	return idx, nil
}

func apiTypeKey(pkg, name string) string { return pkg + "." + name }

func (idx *apiIndex) resolve(ref *manifestRef, current resourceManifest, currentPath string) (string, string) {
	if ref == nil {
		return "", ""
	}
	switch ref.Kind {
	case "well_known":
		return wellKnownTS(ref.Name), ""
	case "message":
		return idx.resolveType(idx.messages, ref, current, currentPath)
	case "enum":
		return idx.resolveType(idx.enums, ref, current, currentPath)
	default:
		return "unknown", ""
	}
}

func (idx *apiIndex) resolveType(table map[string]apiTypeInfo, ref *manifestRef, current resourceManifest, currentPath string) (string, string) {
	if ref.Name == "" {
		return "unknown", ""
	}
	if ref.Package == current.GoPackage {
		return ref.Name, ""
	}
	info, ok := table[apiTypeKey(ref.Package, ref.Name)]
	if !ok {
		return "unknown", ""
	}
	if info.filePath == currentPath {
		return info.name, ""
	}
	rel, err := filepath.Rel(filepath.Dir(currentPath), info.filePath)
	if err != nil {
		return "unknown", ""
	}
	rel = strings.TrimSuffix(filepath.ToSlash(rel), ".ts")
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return info.name, rel
}

// generateAPIFiles writes TypeScript request/response types and wrappers
// for proto services that declare HTTP RPCs. Existing files are left
// untouched so hand-written additions survive regeneration.
func generateAPIFiles(manifests []resourceManifest, apiDir string, check bool) (written, skipped int, missing []string, err error) {
	index, err := buildAPIIndex(manifests, apiDir)
	if err != nil {
		return 0, 0, nil, err
	}
	seen := map[string]bool{}
	for _, m := range manifests {
		path, err := apiFilePath(apiDir, m.Source)
		if err != nil {
			return 0, 0, nil, err
		}
		body := renderAPI(m, index, path)
		if body == "" {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		if _, statErr := os.Stat(path); statErr == nil {
			skipped++
			continue
		} else if !os.IsNotExist(statErr) {
			return 0, 0, nil, fmt.Errorf("stat %s: %w", path, statErr)
		}
		if check {
			missing = append(missing, path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return 0, 0, nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return 0, 0, nil, fmt.Errorf("write %s: %w", path, err)
		}
		written++
	}
	return written, skipped, missing, nil
}

func apiFilePath(apiDir, source string) (string, error) {
	rel := strings.TrimSuffix(filepath.ToSlash(source), ".proto") + ".ts"
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || filepath.IsAbs(clean) ||
		clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid proto source %q", source)
	}
	return filepath.Join(apiDir, clean), nil
}

func renderAPI(m resourceManifest, index *apiIndex, currentPath string) string {
	if len(m.Messages) == 0 && len(m.Enums) == 0 {
		return ""
	}
	hasMethod := false
	for _, svc := range m.Services {
		if len(svc.Methods) > 0 {
			hasMethod = true
			break
		}
	}

	imports := map[string]map[string]bool{}
	var body strings.Builder

	enums := append([]manifestEnum(nil), m.Enums...)
	sort.Slice(enums, func(i, j int) bool { return enums[i].GoName < enums[j].GoName })
	for _, e := range enums {
		if e.GoName == "" {
			continue
		}
		numbers := make([]int32, 0, len(e.Values))
		seenNumbers := map[int32]bool{}
		for _, v := range e.Values {
			if !seenNumbers[v.Number] {
				seenNumbers[v.Number] = true
				numbers = append(numbers, v.Number)
			}
		}
		if len(numbers) == 0 {
			fmt.Fprintf(&body, "export type %s = number\n\n", e.GoName)
		} else {
			parts := make([]string, 0, len(numbers))
			for _, n := range numbers {
				parts = append(parts, fmt.Sprint(n))
			}
			fmt.Fprintf(&body, "export type %s = %s\n\n", e.GoName, strings.Join(parts, " | "))
		}
		fmt.Fprintf(&body, "export const %s = {\n", e.GoName)
		for _, v := range e.Values {
			fmt.Fprintf(&body, "  %s: %d,\n", v.Name, v.Number)
		}
		fmt.Fprintln(&body, "} as const")
		fmt.Fprintln(&body)
	}

	messages := append([]manifestMessage(nil), m.Messages...)
	sort.Slice(messages, func(i, j int) bool { return messages[i].GoName < messages[j].GoName })
	for _, msg := range messages {
		if msg.GoName == "" || msg.GoName == "Empty" {
			continue
		}
		fmt.Fprintf(&body, "export interface %s {\n", msg.GoName)
		for _, f := range msg.Fields {
			if f.Name == "" {
				continue
			}
			typ := fieldTSType(f, m, index, currentPath, imports)
			fmt.Fprintf(&body, "  %s?: %s\n", f.Name, typ)
		}
		fmt.Fprintln(&body, "}")
		fmt.Fprintln(&body)
	}

	nameCounts := map[string]int{}
	for _, svc := range m.Services {
		for _, method := range svc.Methods {
			nameCounts[lowerFirst(method.GoName)]++
		}
	}

	for _, svc := range m.Services {
		for _, method := range svc.Methods {
			name := lowerFirst(method.GoName)
			if nameCounts[name] > 1 {
				name = lowerFirst(svc.GoName) + method.GoName
			}
			fmt.Fprintf(&body, "/** %s %s */\n", method.HTTPMethod, method.HTTPPath)
			reqType := method.Request
			if method.RequestRef != nil {
				if resolved, importPath := index.resolve(method.RequestRef, m, currentPath); resolved != "" {
					reqType = resolved
					addImport(imports, importPath, resolved)
				}
			}
			if reqType == "" {
				reqType = "void"
			}
			respType := method.Response
			if method.ResponseRef != nil {
				if resolved, importPath := index.resolve(method.ResponseRef, m, currentPath); resolved != "" {
					respType = resolved
					addImport(imports, importPath, resolved)
				}
			}
			if respType == "" {
				respType = "void"
			}
			params := ""
			if reqType != "void" {
				params = "req: " + reqType
			}
			fmt.Fprintf(&body, "export function %s(%s): Promise<%s> {\n", name, params, respType)

			pathExpr := renderPath(method.HTTPPath)
			fmt.Fprintf(&body, "  const path = `%s`\n", pathExpr)

			switch strings.ToUpper(method.HTTPMethod) {
			case "GET":
				fields := messageFields(m, reqType)
				pathParams := pathParamNames(method.HTTPPath)
				if query := renderQuery(fields, pathParams, m, index, currentPath, imports); query != "" {
					body.WriteString(query)
					fmt.Fprintln(&body, "  const query = params.toString()")
					fmt.Fprintf(&body, "  return http.get<%s>(`${path}${query ? `?${query}` : ''}`)\n", respType)
				} else {
					fmt.Fprintf(&body, "  return http.get<%s>(path)\n", respType)
				}
			default:
				call := strings.ToLower(method.HTTPMethod)
				args := ""
				if reqType != "void" {
					if method.Body == "*" {
						args = ", req"
					} else if method.Body != "" {
						args = ", req." + method.Body
					}
				}
				fmt.Fprintf(&body, "  return http.%s<%s>(path%s)\n", call, respType, args)
			}
			fmt.Fprintln(&body, "}")
			fmt.Fprintln(&body)
		}
	}

	var out strings.Builder
	fmt.Fprintln(&out, "// Code generated by aeus gen. DO NOT EDIT.")
	fmt.Fprintln(&out)
	if hasMethod {
		fmt.Fprintln(&out, "import { http } from '@/api/http'")
	}
	if len(imports) > 0 {
		fmt.Fprintln(&out)
		paths := make([]string, 0, len(imports))
		for p := range imports {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			names := make([]string, 0, len(imports[p]))
			for name := range imports[p] {
				names = append(names, name)
			}
			sort.Strings(names)
			fmt.Fprintf(&out, "import type { %s } from '%s'\n", strings.Join(names, ", "), p)
		}
	}
	fmt.Fprintln(&out)
	out.WriteString(body.String())
	return out.String()
}

func fieldTSType(f manifestField, m resourceManifest, index *apiIndex, currentPath string, imports map[string]map[string]bool) string {
	base := fieldBaseType(f, m, index, currentPath, imports)
	if f.Map {
		return "Record<string, " + base + ">"
	}
	if f.List {
		return base + "[]"
	}
	return base
}

func fieldBaseType(f manifestField, m resourceManifest, index *apiIndex, currentPath string, imports map[string]map[string]bool) string {
	base := f.Type
	if base == "" {
		base = "unknown"
	}
	if f.Ref == nil {
		return base
	}
	resolved, importPath := index.resolve(f.Ref, m, currentPath)
	if resolved == "" {
		resolved = "unknown"
	}
	addImport(imports, importPath, resolved)
	return resolved
}

func addImport(imports map[string]map[string]bool, importPath, name string) {
	if importPath == "" || name == "" || name == "unknown" {
		return
	}
	if imports[importPath] == nil {
		imports[importPath] = map[string]bool{}
	}
	imports[importPath][name] = true
}

func wellKnownTS(name string) string {
	switch name {
	case "google.protobuf.Timestamp", "google.protobuf.Duration":
		return "{ seconds?: number; nanos?: number }"
	case "google.protobuf.FieldMask":
		return "{ paths?: string[] }"
	case "google.protobuf.StringValue", "google.protobuf.BytesValue":
		return "{ value?: string }"
	case "google.protobuf.BoolValue":
		return "{ value?: boolean }"
	case "google.protobuf.Int32Value", "google.protobuf.Int64Value",
		"google.protobuf.UInt32Value", "google.protobuf.UInt64Value",
		"google.protobuf.FloatValue", "google.protobuf.DoubleValue":
		return "{ value?: number }"
	default:
		return "unknown"
	}
}

func messageFields(m resourceManifest, name string) []manifestField {
	for _, msg := range m.Messages {
		if msg.GoName == name {
			return msg.Fields
		}
	}
	return nil
}

func pathParamNames(path string) map[string]bool {
	out := map[string]bool{}
	for _, match := range pathParamRE.FindAllStringSubmatch(path, -1) {
		out[match[1]] = true
	}
	return out
}

func renderPath(path string) string {
	return pathParamRE.ReplaceAllStringFunc(path, func(token string) string {
		name := strings.TrimPrefix(token, ":")
		return "${encodeURIComponent(String(req." + name + "))}"
	})
}

func renderQuery(fields []manifestField, pathParams map[string]bool, m resourceManifest, index *apiIndex, currentPath string, imports map[string]map[string]bool) string {
	var b strings.Builder
	any := false
	for _, f := range fields {
		if pathParams[f.Name] || f.Map {
			continue
		}
		base := fieldBaseType(f, m, index, currentPath, imports)
		if base != "string" && base != "number" && base != "boolean" {
			continue
		}
		if !any {
			fmt.Fprintln(&b, "  const params = new URLSearchParams()")
			any = true
		}
		if f.List {
			fmt.Fprintf(&b, "  if (Array.isArray(req.%s)) req.%s.forEach((v) => params.append(%q, String(v)))\n", f.Name, f.Name, f.Name)
			fmt.Fprintf(&b, "  else if (req.%s !== undefined && req.%s !== null) params.set(%q, String(req.%s))\n", f.Name, f.Name, f.Name, f.Name)
		} else {
			fmt.Fprintf(&b, "  if (req.%s !== undefined && req.%s !== null) params.set(%q, String(req.%s))\n", f.Name, f.Name, f.Name, f.Name)
		}
	}
	return b.String()
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
