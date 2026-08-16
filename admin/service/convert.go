package service

import (
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/rest/v3"
)

// optionToPb flattens rest dropdown rows to the wire format. nil/empty
// input returns nil so json omits the field instead of producing [].
func optionToPb(in []*rest.TypeValue[string]) []*pb.OptionEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]*pb.OptionEntry, 0, len(in))
	for _, t := range in {
		out = append(out, &pb.OptionEntry{Value: t.Value, Label: t.Label})
	}
	return out
}

// optionNodesToPb walks the rest tier tree and emits the wire OptionNode.
// The recursion is a single helper to keep the wrapper purely structural.
func optionNodesToPb(in []*rest.TierValue[string]) []*pb.OptionNode {
	if len(in) == 0 {
		return nil
	}
	out := make([]*pb.OptionNode, 0, len(in))
	for _, t := range in {
		out = append(out, optionNodeToPb(t))
	}
	return out
}

func optionNodeToPb(t *rest.TierValue[string]) *pb.OptionNode {
	return &pb.OptionNode{
		Value:    t.Value,
		Label:    t.Label,
		Parent:   t.Parent,
		Children: optionNodesToPb(t.Children),
	}
}

// menuNodesToPb mirrors the models.MenuTreeNode -> pb.MenuNode bridge.
// Pure copy; nested Children is recursively converted.
func menuNodesToPb(in []models.MenuTreeNode) []*pb.MenuNode {
	if len(in) == 0 {
		return nil
	}
	out := make([]*pb.MenuNode, 0, len(in))
	for _, n := range in {
		out = append(out, menuNodeToPb(n))
	}
	return out
}

func menuNodeToPb(n models.MenuTreeNode) *pb.MenuNode {
	return &pb.MenuNode{
		// MenuTreeNode.Name was removed in Task 2 — the new
		// identifier field is Component.  The wire-level Name stays
		// the same (downstream consumers expect it), so we map
		// MenuTreeNode.Component -> pb.MenuNode.Name.
		Name:      n.Component,
		Title:     n.Title,
		Component: n.Component,
		Uri:       n.Uri,
		Icon:      n.Icon,
		Hidden:    n.Hidden,
		Public:    n.Public,
		Children:  menuNodesToPb(n.Children),
	}
}

// menusToProtoList flattens []models.Menu (used by VisibleByRole) into
// the response shape. Fields that don't exist on Menu (e.g. nothing
// extra here) are simply left at the proto's zero value.
//
// ViewPath is copied from Menu.ViewPath — populated by
// Server.fillDerivedSpec at Setup time, or by an operator via the
// menu management UI.  An empty ViewPath here means the frontend
// should fall back to its MENU_VIEW_MAP static table for legacy
// components.
func menusToProtoList(in []models.Menu) *pb.ListVisibleMenusResponse {
	if len(in) == 0 {
		return &pb.ListVisibleMenusResponse{Menus: nil}
	}
	items := make([]*pb.MenuEntry, 0, len(in))
	for i := range in {
		m := in[i]
		items = append(items, &pb.MenuEntry{
			Name:      m.Name,
			Component: m.Component,
			Uri:       m.Uri,
			Parent:    m.Parent,
			Icon:      m.Icon,
			Public:    m.Public,
			Hidden:    m.Hidden,
			ViewPath:  m.ViewPath,
		})
	}
	return &pb.ListVisibleMenusResponse{
		TotalCount: int64(len(items)),
		Menus:      items,
	}
}

// PermissionTypeFromPb converts the proto enum to the models-layer
// string constant. Unknown values map to PermissionTypeUnspec so a
// future proto addition (without a models update) does not panic —
// it just returns "all" semantics, which is the safest fallback.
func PermissionTypeFromPb(in pb.PermissionType) models.PermissionType {
	switch in {
	case pb.PermissionType_PERMISSION_TYPE_API:
		return models.PermissionTypeAPI
	case pb.PermissionType_PERMISSION_TYPE_BUTTON:
		return models.PermissionTypeButton
	case pb.PermissionType_PERMISSION_TYPE_DATA_SCOPE:
		return models.PermissionTypeData
	default:
		// PERMISSION_TYPE_MENU no longer maps to anything — menus
		// are not in the catalog.  It falls through to Unspec,
		// which by ListByType semantics means "all types".
		return models.PermissionTypeUnspec
	}
}
