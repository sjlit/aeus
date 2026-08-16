package service

import (
	"testing"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/rest/v3"
)

func TestOptionToPb(t *testing.T) {
	in := []*rest.TypeValue[string]{
		{Value: "admin", Label: "Admin"},
		{Value: "user", Label: "User"},
	}
	out := optionToPb(in)
	if len(out) != 2 {
		t.Fatalf("got %d, want 2", len(out))
	}
	if out[0].Value != "admin" || out[0].Label != "Admin" {
		t.Fatalf("entry 0 wrong: %+v", out[0])
	}
}

func TestOptionNodesToPb(t *testing.T) {
	in := []*rest.TierValue[string]{
		{Value: "system", Label: "System", Children: []*rest.TierValue[string]{
			{Value: "user", Label: "User"},
		}},
	}
	out := optionNodesToPb(in)
	if len(out) != 1 {
		t.Fatalf("got %d, want 1 root", len(out))
	}
	if len(out[0].Children) != 1 || out[0].Children[0].Value != "user" {
		t.Fatalf("child missing or wrong: %+v", out[0])
	}
}

func TestMenuNodesToPb(t *testing.T) {
	in := []models.MenuTreeNode{
		{Component: "sys", Title: "sys", Hidden: true, Children: []models.MenuTreeNode{
			{Component: "user", Title: "user"},
		}},
	}
	out := menuNodesToPb(in)
	if len(out) != 1 || out[0].Name != "sys" {
		t.Fatalf("root mismatch: %+v", out)
	}
	if !out[0].Hidden {
		t.Fatalf("Hidden should propagate")
	}
	if len(out[0].Children) != 1 || out[0].Children[0].Name != "user" {
		t.Fatalf("child mismatch: %+v", out[0])
	}
}

func TestMenusToProtoList(t *testing.T) {
	in := []models.Menu{{Name: "user", Component: "User", Uri: "/user", Icon: "i"}}
	out := menusToProtoList(in)
	if out == nil || len(out.Menus) != 1 {
		t.Fatalf("got %+v, want 1 item", out)
	}
	if out.Menus[0].Component != "User" {
		t.Fatalf("entry wrong: %+v", out.Menus[0])
	}
}

func TestMenusToProtoList_ViewPathPropagates(t *testing.T) {
	// ViewPath populated by Server.fillDerivedSpec at Setup time
	// should reach the wire unchanged.  An empty ViewPath stays empty
	// so the frontend's fallback chain can decide what to do.
	cases := []struct {
		name string
		in   models.Menu
		want string
	}{
		{"auto-derived", models.Menu{Name: "u", Component: "U", ViewPath: "@/views/system/user/Index.vue"}, "@/views/system/user/Index.vue"},
		{"operator-overridden", models.Menu{Name: "u", Component: "U", ViewPath: "@/custom/path/Index.vue"}, "@/custom/path/Index.vue"},
		{"empty fallback signal", models.Menu{Name: "u", Component: "U", ViewPath: ""}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := menusToProtoList([]models.Menu{c.in})
			if got := out.Menus[0].ViewPath; got != c.want {
				t.Errorf("ViewPath = %q, want %q", got, c.want)
			}
		})
	}
}

func TestPermissionTypeFromPb(t *testing.T) {
	cases := []struct {
		in   pb.PermissionType
		want models.PermissionType
	}{
		{pb.PermissionType_PERMISSION_TYPE_MENU, models.PermissionTypeUnspec}, // menus are not in the catalog; falls through to Unspec
		{pb.PermissionType_PERMISSION_TYPE_API, models.PermissionTypeAPI},
		{pb.PermissionType_PERMISSION_TYPE_BUTTON, models.PermissionTypeButton},
		{pb.PermissionType_PERMISSION_TYPE_DATA_SCOPE, models.PermissionTypeData},
		{pb.PermissionType_PERMISSION_TYPE_UNSPECIFIED, models.PermissionTypeUnspec},
		{pb.PermissionType(99), models.PermissionTypeUnspec}, // unknown -> unspec
	}
	for _, c := range cases {
		if got := PermissionTypeFromPb(c.in); got != c.want {
			t.Errorf("FromPb(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
