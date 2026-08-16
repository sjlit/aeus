package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/sjlit/aeus/admin/dbcache"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/infra/cache"
	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/rest/v3"
	"gorm.io/gorm"
)

// MenuServiceOptions holds the dependencies for MenuService.
type MenuServiceOptions struct {
	DB *gorm.DB
	// Cache is the optional shared cache.Cache backend. When non-nil,
	// every read-through Cacher in MenuService routes its entries
	// through it; a nil value falls back to per-Cacher in-memory caches
	// (dbcache.New's default).
	Cache cache.Cache
}

// MenuServiceOption mutates MenuServiceOptions.
type MenuServiceOption func(*MenuServiceOptions)

// MenuService implements the menu RPCs: tree, dropdown options, and
// breadcrumb.
type MenuService struct {
	opts *MenuServiceOptions
	// menusCacher is the read-through cache for MenuTree and
	// MenuBreadcrumb. Built in NewMenuService when opts.DB is set;
	// nil otherwise (the methods fall back to direct DB queries).
	menusCacher *dbcache.Cacher
}

// WithMenuServiceDB wires the gorm handle used by every RPC.
func WithMenuServiceDB(db *gorm.DB) MenuServiceOption {
	return func(o *MenuServiceOptions) { o.DB = db }
}

// WithMenuServiceCache wires a shared cache.Cache backend so multiple
// services can route entries through one store (e.g. a single Redis
// instance). A nil cache keeps dbcache's default in-memory backend.
func WithMenuServiceCache(c cache.Cache) MenuServiceOption {
	return func(o *MenuServiceOptions) { o.Cache = c }
}

// NewMenuService builds a MenuService from the supplied options.
func NewMenuService(opts ...MenuServiceOption) *MenuService {
	o := &MenuServiceOptions{}
	for _, fn := range opts {
		fn(o)
	}
	s := &MenuService{opts: o}
	if o.DB != nil {
		cOpts := []dbcache.Option{
			// SUM(id) on the live (non-soft-deleted) rows: inserts and
			// deletes change the id sum, so CRUD through rest/v3 is
			// caught on the next revalidation.  Pure renames leave the
			// id sum alone, so they ride the 1m TTL — acceptable for
			// admin-only menu edits.  (MAX(updated_at) on Unix-second
			// columns misses same-second writes, which a fast test
			// collides with; SUM(id) dodges that blind spot the same
			// way role_permissions does.)
			dbcache.WithDependency(dbcache.NewSqlDependency(
				dbcache.WithTable((&models.Menu{}).TableName()),
				dbcache.WithColumn("SUM(id)"),
				dbcache.WithCondition("deleted_at IS NULL"),
			)),
			dbcache.WithCacheDuration(time.Minute),
		}
		if o.Cache != nil {
			cOpts = append(cOpts, dbcache.WithCache(o.Cache))
		}
		s.menusCacher = dbcache.New(o.DB, cOpts...)
	}
	return s
}

// MenuTree returns every menu ordered parent-then-id, nested into a
// tree via models.Menu.BuildTree.
func (s *MenuService) MenuTree(ctx context.Context, _ *pb.Empty) (*pb.MenuTreeResponse, error) {
	if s.menusCacher == nil {
		return s.menuTreeDirect(ctx)
	}
	all, err := dbcache.Try(s.menusCacher, ctx, "menu:tree", func(tx *gorm.DB) ([]models.Menu, error) {
		var rows []models.Menu
		if err := tx.WithContext(ctx).Order("parent ASC, id ASC").Find(&rows).Error; err != nil {
			return nil, err
		}
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	return &pb.MenuTreeResponse{Items: menuNodesToPb((&models.Menu{}).BuildTree(all))}, nil
}

// menuTreeDirect is the cache-bypass path used when no Cacher is
// available (e.g. MenuService was built without a DB).
func (s *MenuService) menuTreeDirect(ctx context.Context) (*pb.MenuTreeResponse, error) {
	if s.opts.DB == nil {
		return nil, errs.Newf(errs.CodeUnavailable, "menu service has no database")
	}
	var all []models.Menu
	if err := s.opts.DB.WithContext(ctx).Order("parent ASC, id ASC").Find(&all).Error; err != nil {
		return nil, err
	}
	return &pb.MenuTreeResponse{Items: menuNodesToPb((&models.Menu{}).BuildTree(all))}, nil
}

// MenuOptions returns the parent-picker tier list keyed on
// Menu.Component (the value a parent dropdown must submit).
func (s *MenuService) MenuOptions(ctx context.Context, _ *pb.Empty) (*pb.MenuOptionsResponse, error) {
	// ModelTiers(parent, label, value): the emitted tier VALUE is the
	// Menu.Component — Menu.Parent stores a Component reference, so a
	// frontend picking a parent from this dropdown must submit a
	// Component. The LABEL stays Menu.Name (the human-readable title).
	tv, err := rest.ModelTiers[string](ctx, s.opts.DB, &models.Menu{}, "", "parent", "name", "component")
	if err != nil {
		return nil, err
	}
	return &pb.MenuOptionsResponse{Items: optionNodesToPb(tv)}, nil
}

// MenuBreadcrumb returns the Component path from the root to the
// menu identified by req.Id.
func (s *MenuService) MenuBreadcrumb(ctx context.Context, req *pb.MenuBreadcrumbRequest) (*pb.MenuBreadcrumbResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if s.menusCacher == nil {
		return s.menuBreadcrumbDirect(ctx, req)
	}
	key := "menu:breadcrumb:" + strconv.FormatUint(uint64(req.Id), 10)
	path, err := dbcache.Try(s.menusCacher, ctx, key, func(tx *gorm.DB) ([]string, error) {
		return (&models.Menu{}).PathTo(tx, ctx, uint(req.Id))
	})
	if err != nil {
		// Mirror user.go: map gorm.ErrRecordNotFound to a fresh
		// *errs.Error so the HTTP wrapper asserts it directly and
		// surfaces code 4004 (NotFound) instead of 1003 (Unavailable).
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.Newf(errs.CodeNotFound, "menu %d not found", req.Id)
		}
		return nil, err
	}
	return &pb.MenuBreadcrumbResponse{Path: path}, nil
}

// menuBreadcrumbDirect is the cache-bypass path used when no Cacher
// is available.
func (s *MenuService) menuBreadcrumbDirect(ctx context.Context, req *pb.MenuBreadcrumbRequest) (*pb.MenuBreadcrumbResponse, error) {
	if s.opts.DB == nil {
		return nil, errs.Newf(errs.CodeUnavailable, "menu service has no database")
	}
	path, err := (&models.Menu{}).PathTo(s.opts.DB, ctx, uint(req.Id))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.Newf(errs.CodeNotFound, "menu %d not found", req.Id)
		}
		return nil, err
	}
	return &pb.MenuBreadcrumbResponse{Path: path}, nil
}
