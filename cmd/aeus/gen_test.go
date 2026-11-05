package main

import (
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderGenerated(t *testing.T) {
	manifests := []resourceManifest{{
		Source:        "shop/v1/order.proto",
		GoPackage:     "example.com/shop/proto/shop/v1",
		GoPackageName: "shopv1",
		Module: &manifestModule{
			Name: "shop", Title: "商城管理", Icon: "Shop", Component: "ShopCenter", Sort: 100,
			VarName: "ShopModule",
		},
		Entities: []manifestEntity{{
			GoName: "Order", ModelName: "OrderModel", Table: "orders", Module: "shop",
		}},
		Services: []manifestService{{
			GoName: "OrderService", PoliciesVar: "OrderServicePolicies",
		}},
	}}
	src, err := renderGenerated("gen", manifests)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("gofmt: %v\n%s", err, src)
	}
	got := string(formatted)
	for _, want := range []string{
		`shopv1 "example.com/shop/proto/shop/v1"`,
		`admin.EnsureModuleMenu(db, shopv1.ShopModule)`,
		`s.RegisterModel(&shopv1.OrderModel{})`,
		`admin.EnsureEndpointPermissions(db, shopv1.OrderServicePolicies...)`,
		`db.AutoMigrate(&shopv1.OrderModel{})`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated output missing %q\n%s", want, got)
		}
	}
}

func TestRenderGeneratedNoManifests(t *testing.T) {
	src, err := renderGenerated("gen", nil)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("gofmt: %v\n%s", err, src)
	}
	got := string(formatted)
	if !strings.Contains(got, "func Register(s *admin.Server, db *gorm.DB) error {") {
		t.Errorf("no-op Register missing:\n%s", got)
	}
	if !strings.Contains(got, "func Migrate(db *gorm.DB) error {") {
		t.Errorf("no-op Migrate missing:\n%s", got)
	}
}

func TestLoadManifests(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "shop", "v1")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "order.proto"), []byte("syntax = \"proto3\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := `{"source":"shop/v1/order.proto","go_package":"example.com/shop/proto/shop/v1","go_package_name":"shopv1"}`
	if err := os.WriteFile(filepath.Join(nested, "order_aeus.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	manifests, err := loadManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) != 1 || manifests[0].GoPackage != "example.com/shop/proto/shop/v1" {
		t.Fatalf("manifests = %+v", manifests)
	}
}

func TestRenderGeneratedTypeOnlyDoesNotImportPackage(t *testing.T) {
	src, err := renderGenerated("gen", []resourceManifest{{
		Source:        "user/v1/user.proto",
		GoPackage:     "example.com/user/v1",
		GoPackageName: "userv1",
		Messages:      []manifestMessage{{GoName: "User", Fields: []manifestField{{Name: "name", Type: "string"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := format.Source(src)
	if err != nil {
		t.Fatalf("gofmt: %v\n%s", err, src)
	}
	if strings.Contains(string(formatted), `"example.com/user/v1"`) {
		t.Fatalf("type-only manifest should not be imported:\n%s", formatted)
	}
}

func TestLoadManifestsRejectsStaleSource(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "shop", "v1")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"source":"shop/v1/order.proto","go_package":"example.com/shop/proto/shop/v1"}`
	if err := os.WriteFile(filepath.Join(nested, "order_aeus.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadManifests(dir); err == nil || !strings.Contains(err.Error(), "stale manifest") {
		t.Fatalf("loadManifests error = %v, want stale manifest", err)
	}
}

func TestVuePathAndRender(t *testing.T) {
	entity := manifestEntity{
		GoName: "Order", ModelName: "OrderModel", Table: "orders", Module: "shop",
		Singular: "order", Plural: "orders", MenuName: "订单管理",
		ViewPath: "@/views/shop/order/Index.vue",
	}
	path, err := vueFilePath("web/src/views", entity)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("web/src/views", "shop", "order", "Index.vue") {
		t.Fatalf("path = %q", path)
	}
	body := renderVue(entity)
	for _, want := range []string{
		`defineOptions({ name: "ShopOrders" })`,
		`|| "订单管理"`,
		`<SchemaViewer module="shop" table="orders" :title="title" />`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("vue output missing %q\n%s", want, body)
		}
	}
}

func TestGenerateVueFilesSkipsExisting(t *testing.T) {
	root := t.TempDir()
	entity := manifestEntity{
		GoName: "Order", Table: "orders", Module: "shop", Singular: "order", Plural: "orders",
	}
	manifests := []resourceManifest{{Entities: []manifestEntity{entity}}}
	written, skipped, missing, err := generateVueFiles(manifests, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if written != 1 || skipped != 0 || len(missing) != 0 {
		t.Fatalf("first run: written=%d skipped=%d missing=%v", written, skipped, missing)
	}
	path, err := vueFilePath(root, entity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom page"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, skipped, missing, err = generateVueFiles(manifests, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if written != 0 || skipped != 1 || len(missing) != 0 {
		t.Fatalf("second run: written=%d skipped=%d missing=%v", written, skipped, missing)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "custom page" {
		t.Fatalf("existing page was overwritten: %s", body)
	}
}

func TestGenerateVueFilesCheckMissing(t *testing.T) {
	root := t.TempDir()
	manifests := []resourceManifest{{Entities: []manifestEntity{{
		GoName: "Order", Table: "orders", Module: "shop", Singular: "order", Plural: "orders",
	}}}}
	_, _, missing, err := generateVueFiles(manifests, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 {
		t.Fatalf("missing = %v", missing)
	}
}

func TestRenderAPI(t *testing.T) {
	m := resourceManifest{
		Source: "shop/v1/order.proto",
		Messages: []manifestMessage{
			{GoName: "GetOrderRequest", Fields: []manifestField{{Name: "id", Type: "number"}}},
			{GoName: "OrderView", Fields: []manifestField{{Name: "id", Type: "number"}, {Name: "order_no", Type: "string"}}},
		},
		Services: []manifestService{{
			GoName: "OrderService",
			Methods: []manifestMethod{{
				GoName: "GetOrder", Request: "GetOrderRequest", Response: "OrderView",
				HTTPMethod: "GET", HTTPPath: "/shop/orders/:id",
			}},
		}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{m}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, err := apiFilePath(root, m.Source)
	if err != nil {
		t.Fatal(err)
	}
	body := renderAPI(m, index, path)
	for _, want := range []string{
		`export interface GetOrderRequest {`,
		`id?: number`,
		`export interface OrderView {`,
		`export function getOrder(req: GetOrderRequest): Promise<OrderView> {`,
		"`/shop/orders/${encodeURIComponent(String(req.id))}`",
		`return http.get<OrderView>(path)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}

func TestRenderAPIQueryAndBody(t *testing.T) {
	m := resourceManifest{
		Source: "shop/v1/order.proto",
		Messages: []manifestMessage{
			{GoName: "ListOrdersRequest", Fields: []manifestField{{Name: "page", Type: "number"}, {Name: "status", Type: "string"}}},
			{GoName: "CreateOrderRequest", Fields: []manifestField{{Name: "order_no", Type: "string"}}},
			{GoName: "OrderView", Fields: []manifestField{{Name: "id", Type: "number"}}},
		},
		Services: []manifestService{{
			GoName: "OrderService",
			Methods: []manifestMethod{
				{GoName: "ListOrders", Request: "ListOrdersRequest", Response: "OrderView", HTTPMethod: "GET", HTTPPath: "/shop/orders"},
				{GoName: "CreateOrder", Request: "CreateOrderRequest", Response: "OrderView", HTTPMethod: "POST", HTTPPath: "/shop/orders", Body: "*"},
			},
		}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{m}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, err := apiFilePath(root, m.Source)
	if err != nil {
		t.Fatal(err)
	}
	body := renderAPI(m, index, path)
	for _, want := range []string{
		`const params = new URLSearchParams()`,
		`params.set("page", String(req.page))`,
		`params.set("status", String(req.status))`,
		"return http.get<OrderView>(`${path}${query ? `?${query}` : ''}`)",
		`return http.post<OrderView>(path, req)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}

func TestGenerateAPIFilesSkipsExisting(t *testing.T) {
	root := t.TempDir()
	m := resourceManifest{
		Source:   "shop/v1/order.proto",
		Messages: []manifestMessage{{GoName: "GetOrderRequest", Fields: []manifestField{{Name: "id", Type: "number"}}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "GetOrder", Request: "GetOrderRequest", Response: "void", HTTPMethod: "GET", HTTPPath: "/shop/orders/:id",
		}}}},
	}
	manifests := []resourceManifest{m}
	written, skipped, missing, err := generateAPIFiles(manifests, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if written != 1 || skipped != 0 || len(missing) != 0 {
		t.Fatalf("first run: written=%d skipped=%d missing=%v", written, skipped, missing)
	}
	path, _ := apiFilePath(root, m.Source)
	if err := os.WriteFile(path, []byte("custom api"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, skipped, missing, err = generateAPIFiles(manifests, root, false)
	if err != nil {
		t.Fatal(err)
	}
	if written != 0 || skipped != 1 || len(missing) != 0 {
		t.Fatalf("second run: written=%d skipped=%d missing=%v", written, skipped, missing)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "custom api" {
		t.Fatalf("existing api file was overwritten: %s", body)
	}
}

func TestRenderAPIEnumsNestedAndWellKnown(t *testing.T) {
	m := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/proto/shop/v1",
		Enums: []manifestEnum{{
			GoName: "OrderStatus",
			Values: []manifestEnumValue{
				{Name: "ORDER_STATUS_UNSPECIFIED", Number: 0},
				{Name: "ORDER_STATUS_DRAFT", Number: 1},
			},
		}},
		Messages: []manifestMessage{
			{GoName: "Outer", Fields: []manifestField{{
				Name: "inner", Type: "Outer_Inner",
				Ref: &manifestRef{Kind: "message", Package: "example.com/shop/proto/shop/v1", Name: "Outer_Inner"},
			}}},
			{GoName: "Outer_Inner", Fields: []manifestField{
				{Name: "status", Type: "OrderStatus", Ref: &manifestRef{Kind: "enum", Package: "example.com/shop/proto/shop/v1", Name: "OrderStatus"}},
				{Name: "created_at", Type: "unknown", Ref: &manifestRef{Kind: "well_known", Name: "google.protobuf.Timestamp"}},
			}},
		},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "GetOrder", Request: "Outer", Response: "Outer", HTTPMethod: "GET", HTTPPath: "/shop/orders",
		}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{m}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, m.Source)
	body := renderAPI(m, index, path)
	for _, want := range []string{
		`export type OrderStatus = 0 | 1`,
		`ORDER_STATUS_DRAFT: 1`,
		`export interface Outer {`,
		`inner?: Outer_Inner`,
		`export interface Outer_Inner {`,
		`status?: OrderStatus`,
		`created_at?: { seconds?: number; nanos?: number }`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}

func TestRenderAPICrossFileImport(t *testing.T) {
	order := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/proto/shop/v1",
		Messages: []manifestMessage{{GoName: "OrderView", Fields: []manifestField{{
			Name: "user", Type: "User",
			Ref: &manifestRef{Kind: "message", Package: "example.com/user/proto/user/v1", Name: "User"},
		}}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "GetOrder", Request: "OrderView", Response: "OrderView", HTTPMethod: "GET", HTTPPath: "/shop/orders",
		}}}},
	}
	user := resourceManifest{
		Source:    "user/v1/user.proto",
		GoPackage: "example.com/user/proto/user/v1",
		Messages:  []manifestMessage{{GoName: "User", Fields: []manifestField{{Name: "name", Type: "string"}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{order, user}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, order.Source)
	body := renderAPI(order, index, path)
	for _, want := range []string{
		`import type { User } from '../../user/v1/user'`,
		`user?: User`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}

func TestRenderAPITypeOnlyFile(t *testing.T) {
	m := resourceManifest{
		Source:    "user/v1/user.proto",
		GoPackage: "example.com/user/proto/user/v1",
		Messages:  []manifestMessage{{GoName: "User", Fields: []manifestField{{Name: "name", Type: "string"}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{m}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, m.Source)
	body := renderAPI(m, index, path)
	if !strings.Contains(body, "export interface User") {
		t.Fatalf("type-only interface missing:\n%s", body)
	}
	if strings.Contains(body, "@/api/http") {
		t.Fatalf("type-only file should not import http:\n%s", body)
	}
}

func TestRenderVueUsesExplicitComponentAndReadonly(t *testing.T) {
	body := renderVue(manifestEntity{
		Module: "shop", Table: "order", Component: "ShopOrder",
		MenuName: "订单管理", ReadOnly: true,
	})
	for _, want := range []string{
		`defineOptions({ name: "ShopOrder" })`,
		`<SchemaViewer module="shop" table="order" :title="title" readonly />`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("vue output missing %q\n%s", want, body)
		}
	}
}

func TestRenderAPISamePackageCrossFileImport(t *testing.T) {
	order := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/v1",
		Messages: []manifestMessage{{GoName: "OrderView", Fields: []manifestField{{
			Name: "user", Type: "User",
			Ref: &manifestRef{Kind: "message", Package: "example.com/shop/v1", Name: "User"},
		}}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "GetOrder", Request: "OrderView", Response: "OrderView",
			HTTPMethod: "GET", HTTPPath: "/shop/order",
		}}}},
	}
	user := resourceManifest{
		Source:    "shop/v1/user.proto",
		GoPackage: "example.com/shop/v1",
		Messages:  []manifestMessage{{GoName: "User", Fields: []manifestField{{Name: "name", Type: "string"}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{order, user}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, order.Source)
	body := renderAPI(order, index, path)
	for _, want := range []string{
		`import type { User } from './user'`,
		`user?: User`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}

func TestRenderAPIServiceOnly(t *testing.T) {
	types := resourceManifest{
		Source:    "types/v1/types.proto",
		GoPackage: "example.com/types/v1",
		Messages: []manifestMessage{
			{GoName: "Req", Fields: []manifestField{{Name: "id", Type: "string"}}},
			{GoName: "Res", Fields: []manifestField{{Name: "id", Type: "string"}}},
		},
	}
	service := resourceManifest{
		Source:    "service/v1/service.proto",
		GoPackage: "example.com/service/v1",
		Services: []manifestService{{GoName: "ShopService", Methods: []manifestMethod{{
			GoName: "Get", Request: "Req", Response: "Res",
			RequestRef:  &manifestRef{Kind: "message", Package: "example.com/types/v1", Name: "Req"},
			ResponseRef: &manifestRef{Kind: "message", Package: "example.com/types/v1", Name: "Res"},
			HTTPMethod:  "GET", HTTPPath: "/shop/get",
		}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{types, service}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, service.Source)
	body := renderAPI(service, index, path)
	for _, want := range []string{
		`import type { Req, Res } from '../../types/v1/types'`,
		`export function get(req: Req): Promise<Res>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}

func TestRenderAPIReservedFunctionName(t *testing.T) {
	m := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/v1",
		Messages:  []manifestMessage{{GoName: "Req", Fields: []manifestField{{Name: "id", Type: "number"}}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "Delete", Request: "Req", Response: "void",
			RequestRef: &manifestRef{Kind: "message", Package: "example.com/shop/v1", Name: "Req"},
			HTTPMethod: "DELETE", HTTPPath: "/shop/orders/:id",
		}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{m}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, m.Source)
	body := renderAPI(m, index, path)
	if !strings.Contains(body, `export function rpcDelete(req: Req)`) {
		t.Fatalf("reserved method name not sanitized:\n%s", body)
	}
	if strings.Contains(body, `export function delete(`) {
		t.Fatalf("invalid reserved function emitted:\n%s", body)
	}
}

func TestRenderAPILocalEmptyMessage(t *testing.T) {
	m := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/v1",
		Messages:  []manifestMessage{{GoName: "Empty", Fields: []manifestField{{Name: "name", Type: "string"}}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "GetEmpty", Request: "Empty", Response: "Empty",
			RequestRef:  &manifestRef{Kind: "message", Package: "example.com/shop/v1", Name: "Empty"},
			ResponseRef: &manifestRef{Kind: "message", Package: "example.com/shop/v1", Name: "Empty"},
			HTTPMethod:  "GET", HTTPPath: "/shop/empty",
		}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{m}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, m.Source)
	body := renderAPI(m, index, path)
	for _, want := range []string{
		`export interface Empty {`,
		`export function getEmpty(req: Empty): Promise<Empty>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}

func TestGenerateAPIFilesUpdatesGeneratedFile(t *testing.T) {
	root := t.TempDir()
	manifest := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/v1",
		Messages:  []manifestMessage{{GoName: "Req", Fields: []manifestField{{Name: "id", Type: "number"}}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "Get", Request: "Req", Response: "void",
			RequestRef: &manifestRef{Kind: "message", Package: "example.com/shop/v1", Name: "Req"},
			HTTPMethod: "GET", HTTPPath: "/shop/order",
		}}}},
	}
	if _, _, _, err := generateAPIFiles([]resourceManifest{manifest}, root, false); err != nil {
		t.Fatal(err)
	}
	manifest.Messages[0].Fields = append(manifest.Messages[0].Fields, manifestField{Name: "order_no", Type: "string"})
	if written, _, _, err := generateAPIFiles([]resourceManifest{manifest}, root, false); err != nil {
		t.Fatal(err)
	} else if written != 1 {
		t.Fatalf("written = %d, want 1", written)
	}
	path, _ := apiFilePath(root, manifest.Source)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "order_no?: string") {
		t.Fatalf("generated TS was not refreshed:\n%s", body)
	}
}

func TestGenerateAPIFilesCheckDetectsDrift(t *testing.T) {
	root := t.TempDir()
	manifest := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/v1",
		Messages:  []manifestMessage{{GoName: "Req", Fields: []manifestField{{Name: "id", Type: "number"}}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "Get", Request: "Req", Response: "void",
			RequestRef: &manifestRef{Kind: "message", Package: "example.com/shop/v1", Name: "Req"},
			HTTPMethod: "GET", HTTPPath: "/shop/order",
		}}}},
	}
	if _, _, _, err := generateAPIFiles([]resourceManifest{manifest}, root, false); err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, manifest.Source)
	body, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(body, []byte("\n// drift\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := generateAPIFiles([]resourceManifest{manifest}, root, true); err == nil ||
		!strings.Contains(err.Error(), "out of date") {
		t.Fatalf("check error = %v, want out of date", err)
	}
}

func TestRenderAPIAliasesDuplicateImportedNames(t *testing.T) {
	order := resourceManifest{
		Source:    "shop/v1/order.proto",
		GoPackage: "example.com/shop/v1",
		Messages: []manifestMessage{{GoName: "OrderView", Fields: []manifestField{
			{Name: "buyer", Type: "User", Ref: &manifestRef{Kind: "message", Package: "example.com/user/v1", Name: "User"}},
			{Name: "seller", Type: "User", Ref: &manifestRef{Kind: "message", Package: "example.com/admin/v1", Name: "User"}},
		}}},
		Services: []manifestService{{GoName: "OrderService", Methods: []manifestMethod{{
			GoName: "GetOrder", Request: "OrderView", Response: "OrderView",
			HTTPMethod: "GET", HTTPPath: "/shop/order",
		}}}},
	}
	user := resourceManifest{
		Source:    "user/v1/user.proto",
		GoPackage: "example.com/user/v1",
		Messages:  []manifestMessage{{GoName: "User", Fields: []manifestField{{Name: "name", Type: "string"}}}},
	}
	adminUser := resourceManifest{
		Source:    "admin/v1/user.proto",
		GoPackage: "example.com/admin/v1",
		Messages:  []manifestMessage{{GoName: "User", Fields: []manifestField{{Name: "name", Type: "string"}}}},
	}
	root := t.TempDir()
	index, err := buildAPIIndex([]resourceManifest{order, user, adminUser}, root)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := apiFilePath(root, order.Source)
	body := renderAPI(order, index, path)
	for _, want := range []string{
		`buyer?: User`,
		`seller?: User_2`,
		`import type { User as User_2 } from '../../admin/v1/user'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api output missing %q\n%s", want, body)
		}
	}
}
