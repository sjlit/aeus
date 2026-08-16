package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/dbcache"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/infra/cache"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// __defaultAvatarMaxLen caps an avatar URL to fit models.User.Avatar (size:1024).
const __defaultAvatarMaxLen = 1024

type (
	// UserServiceOptions holds dependencies for UserService.
	UserServiceOptions struct {
		DB *gorm.DB
		// Cache is the optional shared cache.Cache backend. When
		// non-nil, every read-through Cacher in UserService routes
		// its entries through it; a nil value falls back to
		// per-Cacher in-memory caches (dbcache.New's default).
		Cache cache.Cache
	}

	// UserServiceOption mutates UserServiceOptions.
	UserServiceOption func(*UserServiceOptions)

	// UserService implements UserServiceHttpServer. RPCs that act on the
	// caller (profile / change password / set avatar / list menus / list
	// permissions) read the JWT subject from ctx via auth.ClaimsFromContext.
	UserService struct {
		opts *UserServiceOptions
		// grantsCacher is the read-through cache for the
		// (tenant, role) -> grant set used by ListVisibleMenus and
		// ListPermissionCodes.  Built in NewUserService; the methods
		// fall back to direct DB queries when it is nil.
		grantsCacher *dbcache.Cacher
	}
)

// WithUserServiceDB wires the gorm handle used by every RPC.
func WithUserServiceDB(db *gorm.DB) UserServiceOption {
	return func(opts *UserServiceOptions) {
		opts.DB = db
	}
}

// WithUserServiceCache wires a shared cache.Cache backend so multiple
// services can route entries through one store (e.g. a single Redis
// instance).  A nil cache keeps dbcache's default in-memory backend.
func WithUserServiceCache(c cache.Cache) UserServiceOption {
	return func(opts *UserServiceOptions) {
		opts.Cache = c
	}
}

// NewUserService builds a UserService; it panics when WithUserServiceDB
// has not been supplied, since every RPC needs the DB handle. Failing
// at construction beats a nil-pointer panic on the first request.
func NewUserService(opts ...UserServiceOption) *UserService {
	o := &UserServiceOptions{}
	for _, fn := range opts {
		fn(o)
	}
	if o.DB == nil {
		panic("admin: NewUserService requires WithUserServiceDB")
	}
	s := &UserService{opts: o}
	cOpts := []dbcache.Option{
		// SUM(id) of live rows in sys_role_permissions: grant
		// changes rewrite the table wholesale (soft delete +
		// re-insert), so the id sum moves on every change.  The
		// 1m TTL bounds the rare same-second rewrite that nets
		// out to the same id sum.
		dbcache.WithDependency(dbcache.NewSqlDependency(
			dbcache.WithTable((&models.RolePermission{}).TableName()),
			dbcache.WithColumn("SUM(id)"),
			dbcache.WithCondition("deleted_at IS NULL"),
		)),
		dbcache.WithCacheDuration(time.Minute),
	}
	if o.Cache != nil {
		cOpts = append(cOpts, dbcache.WithCache(o.Cache))
	}
	s.grantsCacher = dbcache.New(o.DB, cOpts...)
	return s
}

// loadByUID fetches the sys_users row owned by the caller. The
// tenant scope is appended automatically by the GORM callbacks
// installed in admin.Server.Setup, keyed off the tenant id carried
// in ctx by the JWT middleware.
func (s *UserService) loadByUID(ctx context.Context, claims *auth.Claims) (*models.User, error) {
	u := &models.User{}
	if err := s.opts.DB.WithContext(ctx).Where("uid = ?", claims.UID).First(u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// profileFromUser flattens a models.User into the wire-level UserProfile.
func profileFromUser(u *models.User) *pb.UserProfile {
	return &pb.UserProfile{
		Uid:         u.UID,
		Username:    u.Username,
		Email:       u.Email,
		Gender:      u.Gender,
		Description: u.Description,
		Avatar:      u.Avatar,
		Role:        u.RoleKey,
		DeptId:      u.DeptID,
	}
}

// ---------- RPCs ----------

// Profile returns the calling user's profile.
func (s *UserService) Profile(ctx context.Context, _ *pb.Empty) (*pb.UserProfile, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrAccessDenied
	}
	u, err := s.loadByUID(ctx, claims)
	if err != nil {
		return nil, err
	}
	return profileFromUser(u), nil
}

// UpdateProfile applies the non-empty fields of req to the calling
// user's row.
func (s *UserService) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UserProfile, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrAccessDenied
	}
	u, err := s.loadByUID(ctx, claims)
	if err != nil {
		return nil, err
	}
	if req.Username != "" {
		u.Username = req.Username
	}
	if req.Email != "" {
		u.Email = req.Email
	}
	if req.Gender != "" {
		u.Gender = req.Gender
	}
	// Description follows the same no-op-on-empty convention as the
	// fields above, so a partial profile update cannot wipe it.
	if req.Description != "" {
		u.Description = req.Description
	}
	if err = s.opts.DB.WithContext(ctx).Save(u).Error; err != nil {
		return nil, err
	}
	return profileFromUser(u), nil
}

// ChangePassword verifies the old password and stores the new one.
func (s *UserService) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrAccessDenied
	}
	u, err := s.loadByUID(ctx, claims)
	if err != nil {
		return nil, err
	}
	if !u.ValidatePassword(req.OldPassword) {
		return nil, ErrInvalidPassword
	}
	if req.NewPassword == req.OldPassword {
		return nil, errs.Newf(errs.CodeInvalid, "new password must differ from the old one")
	}
	// Policy enforcement happens in models.User.BeforeUpdate — the
	// single choke point every password write funnels through — so a
	// weak new password is rejected there with the same Invalid code.
	u.Password = req.NewPassword
	if err = s.opts.DB.WithContext(ctx).Save(u).Error; err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}

// ResetPassword lets a caller whose role is super-admin set another
// user's password.  The role row is re-read on every call (rather than
// trusting a claim), so a demotion takes effect without waiting for
// the caller's token to expire.
func (s *UserService) ResetPassword(ctx context.Context, req *pb.ResetPasswordRequest) (*pb.ResetPasswordResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrAccessDenied
	}
	var role models.Role
	if err := s.opts.DB.WithContext(ctx).Where("key = ?", claims.Role).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrAccessDenied
		}
		return nil, err
	}
	if !role.IsSuper {
		return nil, errs.ErrAccessDenied
	}
	target := &models.User{}
	if err := s.opts.DB.WithContext(ctx).Where("uid = ?", req.TargetUid).First(target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrNotFound
		}
		return nil, err
	}
	target.Password = req.NewPassword
	if err := s.opts.DB.WithContext(ctx).Save(target).Error; err != nil {
		return nil, err
	}
	return &pb.ResetPasswordResponse{TargetUid: target.UID}, nil
}

// SetAvatarByURL replaces the calling user's avatar URL.
func (s *UserService) SetAvatarByURL(ctx context.Context, req *pb.SetAvatarByURLRequest) (*pb.SetAvatarByURLResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrAccessDenied
	}
	url := strings.TrimSpace(req.AvatarUrl)
	if url == "" || len(url) > __defaultAvatarMaxLen {
		return nil, errs.ErrInvalid
	}
	// One UPDATE: the tenant scope and uid predicate pin this to the
	// caller's row, so RowsAffected==0 maps cleanly to ErrNotFound
	// without a follow-up SELECT.
	res2 := s.opts.DB.WithContext(ctx).
		Model(&models.User{}).
		Where("uid = ?", claims.UID).
		Update("avatar", url)
	if res2.Error != nil {
		return nil, res2.Error
	}
	if res2.RowsAffected == 0 {
		return nil, errs.ErrNotFound
	}
	return &pb.SetAvatarByURLResponse{AvatarUrl: url}, nil
}

// visibleMenuData is the cached payload of ListVisibleMenus.  The
// Cacher covers sys_role_permissions; sys_menus updates ride the 1m
// TTL (admin-only edits, infrequent).
type visibleMenuData struct {
	Components []string
	Menus      []models.Menu
}

// ListVisibleMenus returns the menus the calling user's role is
// granted.
func (s *UserService) ListVisibleMenus(ctx context.Context, _ *pb.Empty) (*pb.ListVisibleMenusResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrAccessDenied
	}
	if s.grantsCacher == nil {
		return s.listVisibleMenusDirect(ctx, claims)
	}
	key := "user:visible_menus:" + claims.TenantID + ":" + claims.Role
	data, err := dbcache.Try(s.grantsCacher, ctx, key, func(tx *gorm.DB) (visibleMenuData, error) {
		components, err := (&models.RolePermission{}).MenuPermissionDatas(tx, ctx, claims.Role)
		if err != nil {
			return visibleMenuData{}, err
		}
		if len(components) == 0 {
			return visibleMenuData{}, nil
		}
		menus, err := (&models.Menu{}).VisibleMenusByComponents(tx, ctx, components)
		if err != nil {
			return visibleMenuData{}, err
		}
		return visibleMenuData{Components: components, Menus: menus}, nil
	})
	if err != nil {
		return nil, err
	}
	if data.Components == nil {
		return &pb.ListVisibleMenusResponse{}, nil
	}
	// menusToProtoList is the single Menu -> *pb.MenuEntry builder;
	// any new MenuEntry field must be wired there exactly once.
	return menusToProtoList(data.Menus), nil
}

// listVisibleMenusDirect is the cache-bypass path.
func (s *UserService) listVisibleMenusDirect(ctx context.Context, claims *auth.Claims) (*pb.ListVisibleMenusResponse, error) {
	menuComponents, err := (&models.RolePermission{}).MenuPermissionDatas(s.opts.DB, ctx, claims.Role)
	if err != nil {
		return nil, err
	}
	if len(menuComponents) == 0 {
		return &pb.ListVisibleMenusResponse{}, nil
	}
	menus, err := (&models.Menu{}).VisibleMenusByComponents(s.opts.DB, ctx, menuComponents)
	if err != nil {
		return nil, err
	}
	return menusToProtoList(menus), nil
}

// ListPermissionCodes returns the API permission codes granted to the
// calling user's role.
func (s *UserService) ListPermissionCodes(ctx context.Context, _ *pb.Empty) (*pb.ListPermissionCodesResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrAccessDenied
	}
	if s.grantsCacher == nil {
		return s.listPermissionCodesDirect(ctx, claims)
	}
	key := "user:perm_codes:" + claims.TenantID + ":" + claims.Role
	codes, err := dbcache.Try(s.grantsCacher, ctx, key, func(tx *gorm.DB) ([]string, error) {
		return (&models.RolePermission{}).APIPermissionDatas(tx, ctx, claims.Role)
	})
	if err != nil {
		return nil, err
	}
	if codes == nil {
		codes = []string{}
	}
	return &pb.ListPermissionCodesResponse{
		Permissions: codes,
		TotalCount:  int64(len(codes)),
	}, nil
}

// listPermissionCodesDirect is the cache-bypass path.
func (s *UserService) listPermissionCodesDirect(ctx context.Context, claims *auth.Claims) (*pb.ListPermissionCodesResponse, error) {
	codes, err := (&models.RolePermission{}).APIPermissionDatas(s.opts.DB, ctx, claims.Role)
	if err != nil {
		return nil, err
	}
	if codes == nil {
		codes = []string{}
	}
	return &pb.ListPermissionCodesResponse{
		Permissions: codes,
		TotalCount:  int64(len(codes)),
	}, nil
}
