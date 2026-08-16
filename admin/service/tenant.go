package service

import (
	"context"
	"errors"
	"sort"

	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// TenantServiceOptions holds the dependencies for TenantService.
type TenantServiceOptions struct {
	DB *gorm.DB
}

// TenantServiceOption mutates TenantServiceOptions.
type TenantServiceOption func(*TenantServiceOptions)

// TenantService implements the tenant RPCs that the generic rest/v3
// CRUD on sys_tenants cannot express: the global tenant dropdown and
// the per-tenant info read.  Tenant rows carry no tenant_id column, so
// every query here is cross-tenant by design; the GORM tenant callbacks
// skip the model automatically.
type TenantService struct {
	opts *TenantServiceOptions
}

// WithTenantServiceDB wires the gorm handle used by every RPC.
func WithTenantServiceDB(db *gorm.DB) TenantServiceOption {
	return func(o *TenantServiceOptions) { o.DB = db }
}

// NewTenantService builds a TenantService from the supplied options.
func NewTenantService(opts ...TenantServiceOption) *TenantService {
	o := &TenantServiceOptions{}
	for _, fn := range opts {
		fn(o)
	}
	return &TenantService{opts: o}
}

// ListTenantOptions returns every tenant as a (id, name) pair, sorted
// by name for a stable dropdown order.  The list is deliberately
// unfiltered: tenants are global, and a disabled tenant is still a
// selectable option (status gates login, not listing).
func (s *TenantService) ListTenantOptions(ctx context.Context, _ *pb.Empty) (*pb.ListTenantOptionsResponse, error) {
	var tenants []models.Tenant
	if err := s.opts.DB.WithContext(ctx).Find(&tenants).Error; err != nil {
		return nil, err
	}
	sort.Slice(tenants, func(i, j int) bool { return tenants[i].Name < tenants[j].Name })
	items := make([]*pb.TenantOption, 0, len(tenants))
	for i := range tenants {
		items = append(items, &pb.TenantOption{Id: tenants[i].ID, Name: tenants[i].Name})
	}
	return &pb.ListTenantOptionsResponse{Items: items}, nil
}

// Tenant returns the stable wire shape of the tenant with the given
// id, or ErrNotFound.
func (s *TenantService) Tenant(ctx context.Context, req *pb.TenantRequest) (*pb.TenantInfo, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var t models.Tenant
	if err := s.opts.DB.WithContext(ctx).Where("id = ?", req.Id).First(&t).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrNotFound
		}
		return nil, err
	}
	return &pb.TenantInfo{Id: t.ID, Name: t.Name, Status: t.Status, CreatedAt: t.CreatedAt}, nil
}
