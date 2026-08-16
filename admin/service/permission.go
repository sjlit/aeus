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
