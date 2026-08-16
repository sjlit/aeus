package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/dbcache"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/infra/cache"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// RoleServiceOptions holds the dependencies for RoleService.
type RoleServiceOptions struct {
	DB *gorm.DB
	// Cache is the optional shared cache.Cache backend. When
	// non-nil, every read-through Cacher in RoleService routes its
	// entries through it; a nil value falls back to per-Cacher
	// in-memory caches (dbcache.New's default).
	Cache cache.Cache
}

// RoleServiceOption mutates RoleServiceOptions.
type RoleServiceOption func(*RoleServiceOptions)

// RoleService implements the role-scoped RPCs: reading and replacing a
// role's menu / permission grants, role dropdown options, and menu
// preview.
type RoleService struct {
	opts *RoleServiceOptions
	// grantsCacher covers sys_role_permissions: serves the role's
	// granted menus + apis.
	grantsCacher *dbcache.Cacher
	// rolesCacher covers sys_roles (filtered by tenant): serves the
	// per-tenant role dropdown.
	rolesCacher *dbcache.Cacher
}

// WithRoleServiceDB wires the gorm handle used by every RPC.
func WithRoleServiceDB(db *gorm.DB) RoleServiceOption {
	return func(o *RoleServiceOptions) { o.DB = db }
}

// WithRoleServiceCache wires a shared cache.Cache backend so multiple
// services can route entries through one store (e.g. a single Redis
// instance).  A nil cache keeps dbcache's default in-memory backend.
func WithRoleServiceCache(c cache.Cache) RoleServiceOption {
	return func(o *RoleServiceOptions) { o.Cache = c }
}

// NewRoleService builds a RoleService from the supplied options.
func NewRoleService(opts ...RoleServiceOption) *RoleService {
	o := &RoleServiceOptions{}
	for _, fn := range opts {
		fn(o)
	}
	s := &RoleService{opts: o}
	if o.DB == nil {
		return s
	}
	grantsOpts := []dbcache.Option{
		dbcache.WithDependency(dbcache.NewSqlDependency(
			dbcache.WithTable((&models.RolePermission{}).TableName()),
			dbcache.WithColumn("SUM(id)"),
			dbcache.WithCondition("deleted_at IS NULL"),
		)),
		// 30s TTL: role edits take effect quickly via the marker,
		// and 30s bounds the rare same-second rewrite that nets out
		// to the same id sum.
		dbcache.WithCacheDuration(30 * time.Second),
	}
	rolesOpts := []dbcache.Option{
		// sys_roles: the marker is table-global (any role row's
		// update bumps it).  Per-tenant isolation is provided by
		// the GORM tenant callback installed in admin.Server.Setup
		// — the loader still sees only its own tenant's rows
		// even though the marker covers the whole live set.  The
		// 1m TTL bounds the second-resolution blind spot and the
		// rare cross-tenant over-invalidation.
		dbcache.WithDependency(dbcache.NewSqlDependency(
			dbcache.WithTable((&models.Role{}).TableName()),
			dbcache.WithColumn("MAX(updated_at)"),
			dbcache.WithCondition("deleted_at IS NULL"),
		)),
		dbcache.WithCacheDuration(time.Minute),
	}
	if o.Cache != nil {
		grantsOpts = append(grantsOpts, dbcache.WithCache(o.Cache))
		rolesOpts = append(rolesOpts, dbcache.WithCache(o.Cache))
	}
	s.grantsCacher = dbcache.New(o.DB, grantsOpts...)
	s.rolesCacher = dbcache.New(o.DB, rolesOpts...)
	return s
}

// rolePermissionsData is the cached payload of RolePermissions.  The
// per-type branches slice from this struct; BUTTON / DATA_SCOPE skip
// the cache entirely (the catalog exposes them via
// PermissionService.ListCatalog).
type rolePermissionsData struct {
	Menus []string
	Apis  []string
}

// RolePermissions returns the menu and/or API grants of a role
// according to req.Type; UNSPECIFIED returns both.
func (s *RoleService) RolePermissions(ctx context.Context, req *pb.RolePermissionsRequest) (*pb.RolePermissionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	switch req.Type {
	case pb.PermissionType_PERMISSION_TYPE_BUTTON,
		pb.PermissionType_PERMISSION_TYPE_DATA_SCOPE:
		// No role-level junction query for button / data_scope; the
		// catalog exposes those types via PermissionService.ListCatalog.
		return &pb.RolePermissionsResponse{}, nil
	}
	if s.grantsCacher == nil {
		return s.rolePermissionsDirect(ctx, req)
	}
	claims, _ := auth.ClaimsFromContext(ctx)
	tenantID := ""
	if claims != nil {
		tenantID = claims.TenantID
	}
	key := "role:permissions:" + tenantID + ":" + req.Role
	data, err := dbcache.Try(s.grantsCacher, ctx, key, func(tx *gorm.DB) (rolePermissionsData, error) {
		menus, err := (&models.RolePermission{}).MenuPermissionDatas(tx, ctx, req.Role)
		if err != nil {
			return rolePermissionsData{}, err
		}
		apis, err := (&models.RolePermission{}).APIPermissionDatas(tx, ctx, req.Role)
		if err != nil {
			return rolePermissionsData{}, err
		}
		return rolePermissionsData{Menus: menus, Apis: apis}, nil
	})
	if err != nil {
		return nil, err
	}
	switch req.Type {
	case pb.PermissionType_PERMISSION_TYPE_MENU:
		return &pb.RolePermissionsResponse{Menus: data.Menus}, nil
	case pb.PermissionType_PERMISSION_TYPE_API:
		return &pb.RolePermissionsResponse{Apis: data.Apis}, nil
	default:
		return &pb.RolePermissionsResponse{Menus: data.Menus, Apis: data.Apis}, nil
	}
}

// rolePermissionsDirect is the cache-bypass path used when no Cacher
// is available.
func (s *RoleService) rolePermissionsDirect(ctx context.Context, req *pb.RolePermissionsRequest) (*pb.RolePermissionsResponse, error) {
	switch req.Type {
	case pb.PermissionType_PERMISSION_TYPE_MENU:
		menus, err := (&models.RolePermission{}).MenuPermissionDatas(s.opts.DB, ctx, req.Role)
		if err != nil {
			return nil, err
		}
		return &pb.RolePermissionsResponse{Menus: menus}, nil
	case pb.PermissionType_PERMISSION_TYPE_API:
		apis, err := (&models.RolePermission{}).APIPermissionDatas(s.opts.DB, ctx, req.Role)
		if err != nil {
			return nil, err
		}
		return &pb.RolePermissionsResponse{Apis: apis}, nil
	default:
		menus, err := (&models.RolePermission{}).MenuPermissionDatas(s.opts.DB, ctx, req.Role)
		if err != nil {
			return nil, err
		}
		apis, err := (&models.RolePermission{}).APIPermissionDatas(s.opts.DB, ctx, req.Role)
		if err != nil {
			return nil, err
		}
		return &pb.RolePermissionsResponse{Menus: menus, Apis: apis}, nil
	}
}

// ReplaceRolePermissions validates the role and its referenced menu /
// permission rows, then atomically replaces the role's grants.
func (s *RoleService) ReplaceRolePermissions(ctx context.Context, req *pb.ReplaceRolePermissionsRequest) (*pb.ReplaceRolePermissionsResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	err := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var role models.Role
		if err := tx.Where("`key` = ?", req.Role).First(&role).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.ErrNotFound
			}
			return err
		}
		if role.IsSuper {
			// Super roles are granted the full catalog automatically by
			// Seed's converge pass; letting the UI rewrite them here
			// would either lock the super admin out or be silently
			// healed on the next startup — both worse than an explicit
			// refusal.
			return errs.Newf(errs.CodePermissionDenied,
				"super admin role permissions are managed automatically and cannot be modified")
		}
		missing, err := (&models.RolePermission{}).ValidateMenuData(tx, ctx, req.Menus)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			// errs.Format returns a fresh *errs.Error so the
			// generated HTTP wrapper's direct type assertion picks up
			// code 1001 (Invalid) instead of 1003 (Unavailable).
			return errs.Newf(errs.CodeInvalid, "missing menu components %v", missing)
		}
		missing, err = (&models.RolePermission{}).ValidatePermissionData(tx, ctx, req.Apis)
		if err != nil {
			return err
		}
		if len(missing) > 0 {
			return errs.Newf(errs.CodeInvalid, "missing permission datas %v", missing)
		}
		return (&models.Role{}).ReplacePermissions(tx, ctx, req.Role, req.Menus, req.Apis)
	})
	if err != nil {
		return nil, err
	}
	return &pb.ReplaceRolePermissionsResponse{
		Role:     req.Role,
		Affected: int32(len(req.Menus) + len(req.Apis)),
	}, nil
}

// ListRoleOptions returns the tenant-scoped (key, name) role pairs,
// sorted by key for a stable dropdown order.
func (s *RoleService) ListRoleOptions(ctx context.Context, _ *pb.Empty) (*pb.ListRoleOptionsResponse, error) {
	if s.rolesCacher == nil {
		return s.listRoleOptionsDirect(ctx)
	}
	tenantID := auth.TenantIDFromContext(ctx)
	key := "role:options:" + tenantID
	items, err := dbcache.Try(s.rolesCacher, ctx, key, func(tx *gorm.DB) ([]*pb.OptionEntry, error) {
		// Query sys_roles directly rather than going through
		// rest.ModelTypes: the latter's domainName parameter is a
		// `WHERE domain = ?` filter, and sys_roles has no `domain`
		// column.  The GORM tenant callback installed by
		// admin.Server.Setup appends `tenant_id = ?` to the
		// underlying query; in tests without the callback this
		// returns the whole table, which is fine because the
		// cache key is per-tenant.
		var roles []models.Role
		if err := tx.WithContext(ctx).Find(&roles).Error; err != nil {
			return nil, err
		}
		sort.Slice(roles, func(i, j int) bool { return roles[i].Key < roles[j].Key })
		out := make([]*pb.OptionEntry, 0, len(roles))
		for i := range roles {
			out = append(out, &pb.OptionEntry{Value: roles[i].Key, Label: roles[i].Name})
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListRoleOptionsResponse{Items: items}, nil
}

// listRoleOptionsDirect is the cache-bypass path used when no Cacher
// is available.
func (s *RoleService) listRoleOptionsDirect(ctx context.Context) (*pb.ListRoleOptionsResponse, error) {
	if s.opts.DB == nil {
		return nil, errs.Newf(errs.CodeUnavailable, "role service has no database")
	}
	var roles []models.Role
	if err := s.opts.DB.WithContext(ctx).Find(&roles).Error; err != nil {
		return nil, err
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Key < roles[j].Key })
	out := make([]*pb.OptionEntry, 0, len(roles))
	for i := range roles {
		out = append(out, &pb.OptionEntry{Value: roles[i].Key, Label: roles[i].Name})
	}
	return &pb.ListRoleOptionsResponse{Items: out}, nil
}

// RoleMenus returns the visible menus for a specific role (admin
// preview).  Moved from MenuService (2026-08-10) so every role-scoped
// endpoint lives under /role/*.
func (s *RoleService) RoleMenus(ctx context.Context, req *pb.RoleMenusRequest) (*pb.ListVisibleMenusResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	menus, err := (&models.Menu{}).VisibleByRole(s.opts.DB, ctx, req.Role)
	if err != nil {
		return nil, err
	}
	return menusToProtoList(menus), nil
}
