package service

import (
	"context"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"gorm.io/gorm"
)

// PermissionServiceOptions holds the dependencies for PermissionService.
type PermissionServiceOptions struct {
	DB *gorm.DB
}

// PermissionServiceOption mutates PermissionServiceOptions.
type PermissionServiceOption func(*PermissionServiceOptions)

// PermissionService serves the global permission catalog
// (sys_permissions).
type PermissionService struct {
	opts *PermissionServiceOptions
}

// WithPermissionServiceDB wires the gorm handle used by every RPC.
func WithPermissionServiceDB(db *gorm.DB) PermissionServiceOption {
	return func(o *PermissionServiceOptions) { o.DB = db }
}

// NewPermissionService builds a PermissionService from the supplied options.
func NewPermissionService(opts ...PermissionServiceOption) *PermissionService {
	o := &PermissionServiceOptions{}
	for _, fn := range opts {
		fn(o)
	}
	return &PermissionService{opts: o}
}

// ListCatalog lists the global permission catalog (sys_permissions)
// entries.  With no type filter, returns all types; otherwise filters
// by the proto enum.
func (s *PermissionService) ListCatalog(ctx context.Context, req *pb.ListCatalogRequest) (*pb.ListCatalogResponse, error) {
	items, err := (&models.Permission{}).ListByType(s.opts.DB, ctx, PermissionTypeFromPb(req.Type))
	if err != nil {
		return nil, err
	}
	return &pb.ListCatalogResponse{Items: items}, nil
}

// ListPermissions returns the full catalog rows (id/type/data/
// description/group) for the admin management UI.  An empty req.Type
// means "all types"; otherwise filters by the models-layer
// PermissionType string constant.
//
// Distinct from ListCatalog: ListCatalog returns only Permission.Data
// strings (consumed by the runtime PermissionChecker); this endpoint
// returns the same catalog with description metadata so the UI can
// render labels alongside codes.
//
// Rows are ordered by id ASC — the catalog grows monotonically as
// Setup registers new models, so id order is also "registration
// order", which is what the UI's "audit, login log, ..." grouping
// implicitly relies on.  If you change this, also re-check that the
// front-end's el-collapse ordering (zh-Hans title sort) still feels
// right with the new server order — the UI does NOT trust the wire
// order, so this is purely a debug-time invariant.
func (s *PermissionService) ListPermissions(ctx context.Context, req *pb.ListPermissionsRequest) (*pb.ListPermissionsResponse, error) {
	var rows []models.Permission
	q := s.opts.DB.WithContext(ctx).Order("id ASC")
	if req.Type != "" {
		q = q.Where("type = ?", req.Type)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]*pb.ListPermissionItem, 0, len(rows))
	for i := range rows {
		items = append(items, &pb.ListPermissionItem{
			Id:          int64(rows[i].ID),
			Type:        rows[i].Type,
			Data:        rows[i].Data,
			Description: rows[i].Description,
			Group:       rows[i].Group,
		})
	}
	return &pb.ListPermissionsResponse{Items: items}, nil
}
