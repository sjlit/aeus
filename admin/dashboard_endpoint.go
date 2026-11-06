package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// DashboardEndpointPath is the route mounted by RegisterDashboardEndpoint.
// It returns the signed-in operator's workspace overview: real system counts
// for super roles and a personal-only payload for everyone else.
//
// Path:    GET /dashboard/overview
// Auth:    mounted behind the JWT middleware. The route is part of the
// checker's self-service allowlist because the dashboard is the landing
// page for every signed-in user; the handler itself decides whether the
// caller may see global system counts.
const DashboardEndpointPath = "/dashboard/overview"

// DashboardOverview is the wire payload for GET /dashboard/overview.
//
// Scope is "admin" for super roles (counts/logins/audits are populated) and
// "personal" for everyone else. A personal response deliberately omits the
// system sections instead of returning zeroes, so the frontend can hide
// cards without inventing data.
type DashboardOverview struct {
	Scope       string           `json:"scope"`
	Counts      *DashboardCounts `json:"counts,omitempty"`
	Logins      *DashboardLogins `json:"logins,omitempty"`
	Audits      *DashboardAudits `json:"audits,omitempty"`
	GeneratedAt int64            `json:"generated_at"`
}

// DashboardCounts carries the row counts shown on the overview cards.
// Menus and permissions are global catalog tables; the rest are tenant
// scoped when multi-tenant mode is enabled.
type DashboardCounts struct {
	Users       int64 `json:"users"`
	Roles       int64 `json:"roles"`
	Departments int64 `json:"departments"`
	Menus       int64 `json:"menus"`
	Permissions int64 `json:"permissions"`
	Tenants     int64 `json:"tenants,omitempty"`
}

// DashboardLogins is the login summary for the last seven days. Yesterday
// is included so the frontend can render a real day-over-day delta instead
// of a hard-coded badge.
type DashboardLogins struct {
	Today     int64                 `json:"today"`
	Yesterday int64                 `json:"yesterday"`
	Trend     []DashboardTrendPoint `json:"trend"`
}

// DashboardTrendPoint is one day in the login trend series.
type DashboardTrendPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// DashboardAudits is the operation-audit summary for the current day plus a
// short recent feed for the activity card.
type DashboardAudits struct {
	Today  int64                `json:"today"`
	Recent []DashboardAuditItem `json:"recent"`
}

// DashboardAuditItem is the trimmed audit shape the dashboard needs. It
// deliberately does not expose Audit.Data: the dashboard is a summary, and
// the full diff belongs on the operation-log page.
type DashboardAuditItem struct {
	ID        uint   `json:"id"`
	UID       string `json:"uid"`
	Action    string `json:"action"`
	Module    string `json:"module"`
	Table     string `json:"table"`
	CreatedAt int64  `json:"created_at"`
}

// RegisterDashboardEndpoint wires GET DashboardEndpointPath onto opts.Router
// and returns the registered handler for direct invocation.
func RegisterDashboardEndpoint(opts *Options) (http.HandlerFunc, error) {
	if opts == nil || opts.Router == nil {
		return nil, ErrRouterRequired
	}
	if opts.DB == nil {
		return nil, ErrDBRequired
	}
	h := newDashboardHandler(opts)
	opts.Router.Handle(http.MethodGet, DashboardEndpointPath, h)
	return h, nil
}

func newDashboardHandler(opts *Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.ClaimsFromContext(r.Context()); !ok {
			writeEnvelope(w, int(errs.CodeUnauthorized), "unauthorized", nil)
			return
		}
		overview, err := loadDashboardOverview(r.Context(), opts)
		if err != nil {
			if opts.Logger != nil {
				opts.Logger.Warn(r.Context(), "load dashboard overview failed", "error", err.Error())
			}
			writeEnvelope(w, int(errs.CodeInternal), "dashboard overview unavailable", nil)
			return
		}
		writeEnvelope(w, int(errs.CodeOK), "", overview)
	}
}

// loadDashboardOverview builds the dashboard payload. It is split out of the
// HTTP handler so the data-loading path can be unit tested without a router.
func loadDashboardOverview(ctx context.Context, opts *Options) (*DashboardOverview, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok {
		return nil, errs.ErrUnauthorized
	}
	now := time.Now()
	base := &DashboardOverview{Scope: "personal", GeneratedAt: now.Unix()}

	db := opts.DB.WithContext(ctx)
	super, err := callerIsSuperRole(db, claims.Role)
	if err != nil {
		return nil, err
	}
	if !super {
		return base, nil
	}

	counts, err := loadDashboardCounts(db, opts.MultiTenant)
	if err != nil {
		return nil, err
	}
	logins, err := loadDashboardLogins(db, now)
	if err != nil {
		return nil, err
	}
	audits, err := loadDashboardAudits(db, now)
	if err != nil {
		return nil, err
	}

	base.Scope = "admin"
	base.Counts = counts
	base.Logins = logins
	base.Audits = audits
	return base, nil
}

// callerIsSuperRole resolves the caller's role row and reports whether it is
// a super role. A missing role is treated as non-super, not as an error: the
// JWT can outlive a deleted role and the dashboard should still render.
func callerIsSuperRole(db *gorm.DB, roleKey string) (bool, error) {
	if roleKey == "" {
		return false, nil
	}
	var role models.Role
	err := db.Where("`key` = ?", roleKey).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve dashboard role: %w", err)
	}
	return role.IsSuper, nil
}

func loadDashboardCounts(db *gorm.DB, multiTenant bool) (*DashboardCounts, error) {
	out := &DashboardCounts{}
	if err := db.Model(&models.User{}).Count(&out.Users).Error; err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	if err := db.Model(&models.Role{}).Count(&out.Roles).Error; err != nil {
		return nil, fmt.Errorf("count roles: %w", err)
	}
	if err := db.Model(&models.Department{}).Count(&out.Departments).Error; err != nil {
		return nil, fmt.Errorf("count departments: %w", err)
	}
	if err := db.Model(&models.Menu{}).Count(&out.Menus).Error; err != nil {
		return nil, fmt.Errorf("count menus: %w", err)
	}
	if err := db.Model(&models.Permission{}).Count(&out.Permissions).Error; err != nil {
		return nil, fmt.Errorf("count permissions: %w", err)
	}
	if multiTenant {
		if err := db.Model(&models.Tenant{}).Count(&out.Tenants).Error; err != nil {
			return nil, fmt.Errorf("count tenants: %w", err)
		}
	}
	return out, nil
}

func loadDashboardLogins(db *gorm.DB, now time.Time) (*DashboardLogins, error) {
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startYesterday := startToday.AddDate(0, 0, -1)
	startWindow := startToday.AddDate(0, 0, -6)

	var rows []models.LoginLog
	if err := db.Select("created_at").
		Where("created_at >= ?", startWindow.Unix()).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load login trend: %w", err)
	}

	out := &DashboardLogins{Trend: make([]DashboardTrendPoint, 0, 7)}
	counts := make(map[string]int64, 7)
	for i := 0; i < 7; i++ {
		day := startWindow.AddDate(0, 0, i)
		key := day.Format("2006-01-02")
		out.Trend = append(out.Trend, DashboardTrendPoint{Date: key})
		counts[key] = 0
	}
	for _, row := range rows {
		day := time.Unix(row.CreatedAt, 0).In(now.Location())
		key := day.Format("2006-01-02")
		if _, ok := counts[key]; !ok {
			continue
		}
		counts[key]++
		if row.CreatedAt >= startToday.Unix() {
			out.Today++
		}
		if row.CreatedAt >= startYesterday.Unix() && row.CreatedAt < startToday.Unix() {
			out.Yesterday++
		}
	}
	for i := range out.Trend {
		out.Trend[i].Count = counts[out.Trend[i].Date]
	}
	return out, nil
}

func loadDashboardAudits(db *gorm.DB, now time.Time) (*DashboardAudits, error) {
	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	out := &DashboardAudits{Recent: make([]DashboardAuditItem, 0, 6)}
	if err := db.Model(&models.Audit{}).
		Where("created_at >= ?", startToday.Unix()).
		Count(&out.Today).Error; err != nil {
		return nil, fmt.Errorf("count audits: %w", err)
	}

	var rows []models.Audit
	if err := db.Order("created_at DESC, id DESC").Limit(6).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load recent audits: %w", err)
	}
	for _, row := range rows {
		out.Recent = append(out.Recent, DashboardAuditItem{
			ID:        row.ID,
			UID:       row.UID,
			Action:    row.Action,
			Module:    row.Module,
			Table:     row.Table,
			CreatedAt: row.CreatedAt,
		})
	}
	return out, nil
}
