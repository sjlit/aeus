package service

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/sjlit/aeus/admin/auth"
	"github.com/sjlit/aeus/admin/models"
	"github.com/sjlit/aeus/admin/pb"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/pkg/errs"
	"gorm.io/gorm"
)

// ErrInvalidPassword is returned by Login / ChangePassword when the
// supplied credentials do not match. The message deliberately avoids
// distinguishing "user not found" from "wrong password" so callers
// cannot enumerate usernames.
var ErrInvalidPassword = errs.Newf(errs.CodeAccessDenied, "invalid username or password")

// Status-check errs. They fire only AFTER the password verified, so
// they do not widen the username-enumeration surface; the explicit
// messages follow the "tenant is disabled" precedent.
var (
	ErrUserDisabled = errs.Newf(errs.CodePermissionDenied, "user is disabled")
	ErrRoleDisabled = errs.Newf(errs.CodePermissionDenied, "role is disabled")
	ErrRoleNotFound = errs.Newf(errs.CodePermissionDenied, "role not found")
)

// tokenTypeAccess / tokenTypeRefresh are the values of the token_type
// claim minted by createToken. RefreshToken rejects any token whose
// token_type is not tokenTypeRefresh, so a leaked access token cannot
// be exchanged for a fresh access token.
const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

type (
	// BeforeLoginFunc is invoked after request validation but before the
	// credential check. Returning an error short-circuits Login.
	BeforeLoginFunc func(ctx context.Context, req *pb.LoginRequest) error

	// AfterLoginFunc is invoked after a successful login and token
	// issuance. It is informational only; its return value is ignored.
	AfterLoginFunc func(ctx context.Context, req *pb.LoginRequest, res *pb.LoginResponse)

	// TokenStore is a revocation DENYLIST keyed by JWT id (jti).
	// Revoke marks an id unusable until its natural expiry (logout);
	// Revoked reports whether an id is currently denied;
	// RevokeIfNotRevoked is the atomic variant refresh rotation uses as
	// a replay fence.  Nothing is written on login — the denylist only
	// ever holds explicitly revoked ids.  NewAuthService defaults it to
	// an in-memory store (see WithTokenStore); the JWT middleware
	// consumes it through RevocationValidator.
	TokenStore interface {
		Revoke(ctx context.Context, jti string, until time.Time) error
		Revoked(ctx context.Context, jti string) (bool, error)
		RevokeIfNotRevoked(ctx context.Context, jti string, until time.Time) (bool, error)
	}

	// AuthServiceOptions holds the dependencies and hook configuration
	// for AuthService.
	AuthServiceOptions struct {
		DB                 *gorm.DB
		TokenExpireSeconds int64
		BeforeLogin        BeforeLoginFunc
		AfterLogin         AfterLoginFunc
		TokenStore         TokenStore
		AuthSecret         string
		LoginLogFunc       LoginLogFunc
	}

	// AuthServiceOption mutates AuthServiceOptions.
	AuthServiceOption func(*AuthServiceOptions)

	// AuthService implements the login lifecycle: password login,
	// refresh-token exchange, and logout (token revocation).
	AuthService struct {
		opts      *AuthServiceOptions
		secretKey []byte // pre-cast from opts.AuthSecret so JWT signing/verify don't reallocate
	}
)

// WithAuthServiceDB wires the gorm handle used by Login to look up users.
func WithAuthServiceDB(db *gorm.DB) AuthServiceOption {
	return func(opts *AuthServiceOptions) {
		opts.DB = db
	}
}

// WithAuthSecret sets the secret used to sign and verify JWTs.
// Required for Login / RefreshToken / Logout to succeed.
func WithAuthSecret(secret string) AuthServiceOption {
	return func(opts *AuthServiceOptions) {
		opts.AuthSecret = secret
	}
}

// WithTokenExpireSeconds overrides the access-token TTL. A non-positive
// value is treated as the default (2h) when tokens are issued.
func WithTokenExpireSeconds(seconds int64) AuthServiceOption {
	return func(opts *AuthServiceOptions) {
		opts.TokenExpireSeconds = seconds
	}
}

// WithTokenStore overrides the default in-memory denylist store.  Pass
// a shared or persistent store when revocation must survive process
// restarts or span multiple instances; a nil store falls back to the
// default.  The store is consumed by RevocationValidator (JWT
// middleware side) and by RefreshToken / Logout (service side).
func WithTokenStore(store TokenStore) AuthServiceOption {
	return func(opts *AuthServiceOptions) {
		opts.TokenStore = store
	}
}

// WithBeforeLogin registers a hook fired after Validate() but before
// credential check. Returning an error short-circuits the login.
func WithBeforeLogin(fn BeforeLoginFunc) AuthServiceOption {
	return func(opts *AuthServiceOptions) {
		opts.BeforeLogin = fn
	}
}

// WithAfterLogin registers a hook fired after a successful login and
// token issuance. It is informational only; its return value is ignored.
func WithAfterLogin(fn AfterLoginFunc) AuthServiceOption {
	return func(opts *AuthServiceOptions) {
		opts.AfterLogin = fn
	}
}

// __defaultTokenExpireSeconds is used when TokenExpireSeconds is not set.
const __defaultTokenExpireSeconds int64 = 2 * 60 * 60 // 2h

// __refreshTokenTTL is the lifetime of refresh tokens (48h, in seconds).
const __refreshTokenTTL int64 = 48 * 60 * 60

// ttl returns the configured TTL or the default when not configured / non-positive.
func (s *AuthService) ttl() int64 {
	if s.opts.TokenExpireSeconds > 0 {
		return s.opts.TokenExpireSeconds
	}
	return __defaultTokenExpireSeconds
}

// NewAuthService builds an AuthService. It panics when WithAuthServiceDB
// or a non-empty WithAuthSecret has not been supplied: without a DB the
// service cannot authenticate anyone, and an empty signing secret would
// mint tokens anyone can forge, so both are programming errors that
// should surface at boot rather than at the first request.
func NewAuthService(opts ...AuthServiceOption) *AuthService {
	options := &AuthServiceOptions{}
	for _, o := range opts {
		o(options)
	}
	if options.DB == nil {
		panic("admin: NewAuthService requires WithAuthServiceDB")
	}
	if options.AuthSecret == "" {
		panic("admin: NewAuthService requires a non-empty WithAuthSecret")
	}
	if options.TokenExpireSeconds <= 0 {
		options.TokenExpireSeconds = __defaultTokenExpireSeconds
	}
	// Without an explicit store, revocation falls back to the
	// in-memory default so Logout actually invalidates the token.
	if options.TokenStore == nil {
		options.TokenStore = NewMemoryTokenStore()
	}
	return &AuthService{
		opts:      options,
		secretKey: []byte(options.AuthSecret),
	}
}

// resolveTenant returns the display name and status of the given
// tenant in a single sys_tenants read, shared by Login's status gate
// and the LoginResponse display fields. Backward-compatible fallbacks:
// a missing table (database predating the tenant model) or a missing
// row (legacy orphan tenant ids) yields the tenant id as name and an
// empty status, which Login treats as enabled — pre-existing databases
// keep logging in.
func (s *AuthService) resolveTenant(ctx context.Context, tenantID string) (name, status string) {
	if tenantID == "" || s.opts.DB == nil {
		return tenantID, ""
	}
	db := s.opts.DB.WithContext(ctx)
	if !db.Migrator().HasTable("sys_tenants") {
		return tenantID, ""
	}
	var row struct {
		Name   string
		Status string
	}
	if err := db.Table("sys_tenants").
		Select("name", "status").
		Where("id = ?", tenantID).
		Scan(&row).Error; err != nil || row.Name == "" {
		return tenantID, ""
	}
	return row.Name, row.Status
}

// createToken builds and signs a JWT for the given user/role/tenant.
// tokenType is stored in the token_type claim so RefreshToken can
// distinguish refresh tokens from access tokens. ttl is interpreted as
// SECONDS — callers must not pass nanoseconds.
func (s *AuthService) createToken(userid, role, tenantID, tokenType string, ttl int64) (accessToken string, err error) {
	now := time.Now()
	claims := &auth.Claims{
		UID:       userid,
		Role:      role,
		TenantID:  tenantID,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(ttl) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err = token.SignedString(s.secret())
	return accessToken, err
}

// secret returns the HMAC signing key as []byte. JWT v5 rejects string keys
// for HS256/HS384/HS512, so we centralise the cast here. The cast is
// performed once at NewAuthService time and cached — each call to
// secret() returns the same backing array without re-allocating.
func (s *AuthService) secret() []byte {
	return s.secretKey
}

// keyfunc returns the jwt v5 key function used to parse and verify
// tokens. It pins the signing method to HS256 so a token carrying any
// other alg is rejected before signature verification (algorithm
// confusion defense).
func (s *AuthService) keyfunc() jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret(), nil
	}
}

// Login validates credentials, issues an access + refresh token pair,
// records the attempt via LoginLogFunc, and runs the AfterLogin hook.
func (s *AuthService) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	if err := req.Validate(); err != nil {
		s.recordLogin(ctx, req, false, nil, "")
		return nil, err
	}
	if s.opts.BeforeLogin != nil {
		if err := s.opts.BeforeLogin(ctx, req); err != nil {
			s.recordLogin(ctx, req, false, nil, "")
			return nil, err
		}
	}
	userModel := &models.User{}
	if err := s.opts.DB.WithContext(ctx).Where("uid = ? OR username = ?", req.Username, req.Username).First(userModel).Error; err != nil {
		s.recordLogin(ctx, req, false, nil, "")
		return nil, ErrInvalidPassword
	}
	if !userModel.ValidatePassword(req.Password) {
		s.recordLogin(ctx, req, false, userModel, "")
		return nil, ErrInvalidPassword
	}
	if userModel.Status == "disabled" {
		s.recordLogin(ctx, req, false, userModel, "")
		return nil, ErrUserDisabled
	}
	// Role status gate. Login runs without claims in ctx (the route is
	// on the JWT allowlist), so the GORM tenant callbacks cannot filter
	// here — tenant_id must be scoped explicitly or a same-key role on
	// another tenant would answer this lookup.
	var role models.Role
	err := s.opts.DB.WithContext(ctx).
		Where("key = ? AND tenant_id = ?", userModel.RoleKey, userModel.TenantID).
		First(&role).Error
	if errs.Is(err, gorm.ErrRecordNotFound) {
		s.recordLogin(ctx, req, false, userModel, "")
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, err
	}
	if role.Status == "disabled" {
		s.recordLogin(ctx, req, false, userModel, "")
		return nil, ErrRoleDisabled
	}
	tenantName, tenantStatus := s.resolveTenant(ctx, userModel.TenantID)
	if tenantStatus == "disabled" {
		s.recordLogin(ctx, req, false, userModel, "")
		return nil, errs.Newf(errs.CodePermissionDenied, "tenant is disabled")
	}
	ttl := s.ttl()
	accessToken, err := s.createToken(userModel.UID, userModel.RoleKey, userModel.TenantID, tokenTypeAccess, ttl)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.createToken(userModel.UID, userModel.RoleKey, userModel.TenantID, tokenTypeRefresh, __refreshTokenTTL)
	if err != nil {
		return nil, err
	}
	res := &pb.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Uid:          userModel.UID,
		Username:     userModel.Username,
		Expires:      ttl,
		TenantId:     userModel.TenantID,
		TenantName:   tenantName,
	}

	s.recordLogin(ctx, req, true, userModel, res.AccessToken)
	if s.opts.AfterLogin != nil {
		s.opts.AfterLogin(ctx, req, res)
	}
	return res, nil
}

// recordLogin hands a LoginLogInfo entry to the optional recorder.
// On failed attempts userModel may be nil (lookup failed) or non-nil
// (password failed); only the success path passes a non-empty token.
// Runs synchronously on the login path — recorder latency is on the
// caller. A panicking recorder propagates to Login's caller.
func (s *AuthService) recordLogin(ctx context.Context, req *pb.LoginRequest,
	success bool, userModel *models.User, token string,
) {
	if s.opts.LoginLogFunc == nil {
		return
	}
	var uid, tenantID string
	if userModel != nil {
		uid, tenantID = userModel.UID, userModel.TenantID
	}
	ip, _ := metadata.Get(ctx, metadata.RequestClientIP)
	ua, _ := metadata.Get(ctx, "User-Agent")
	s.opts.LoginLogFunc(ctx, LoginLogInfo{
		Success:     success,
		UID:         uid,
		TenantID:    tenantID,
		Username:    req.Username,
		IP:          ip,
		UserAgent:   ua,
		AccessToken: token,
	})
}

// RefreshToken exchanges a valid refresh token for a fresh access
// token.
func (s *AuthService) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	refreshToken, err := jwt.ParseWithClaims(req.RefreshToken, &auth.Claims{}, s.keyfunc())
	if err != nil {
		return nil, err
	}
	refreshClaims, ok := refreshToken.Claims.(*auth.Claims)
	if !ok {
		return nil, errs.ErrIncompatible
	}
	if refreshClaims.TokenType != tokenTypeRefresh {
		// An access token (or a token minted before the token_type
		// claim existed) is not a valid refresh token.
		return nil, errs.ErrAccessDenied
	}
	// Re-check user and role status on every refresh so a disabled
	// account stops minting access tokens, not just stops logging in.
	// /auth/refresh-token sits on the JWT allowlist — no claims in ctx,
	// so both lookups filter tenant_id explicitly.
	var user models.User
	if err := s.opts.DB.WithContext(ctx).
		Where("uid = ? AND tenant_id = ?", refreshClaims.UID, refreshClaims.TenantID).
		First(&user).Error; err != nil {
		if errs.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrAccessDenied
		}
		return nil, err
	}
	if user.Status == "disabled" {
		return nil, ErrUserDisabled
	}
	var role models.Role
	if err := s.opts.DB.WithContext(ctx).
		Where("key = ? AND tenant_id = ?", user.RoleKey, user.TenantID).
		First(&role).Error; err != nil {
		if errs.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrAccessDenied
		}
		return nil, err
	}
	if role.Status == "disabled" {
		return nil, ErrRoleDisabled
	}
	// Rotate: atomically claim the old jti BEFORE minting anything —
	// RevokeIfNotRevoked serializes concurrent refreshes of the same
	// token (exactly one wins; losers are replays or logged-out tokens
	// and get 4005).  The winner's new refresh token carries a fresh
	// jti; the old one stays denylisted until its natural expiry.
	//
	// Tokens minted before jti existed (ID == "") skip the fence: they
	// still receive a rotated pair, but the old token stays valid until
	// its natural expiry and cannot be replay-detected.
	ttl := s.ttl()
	if s.opts.TokenStore != nil && refreshClaims.ID != "" {
		until := time.Now().Add(time.Duration(__refreshTokenTTL) * time.Second)
		if refreshClaims.ExpiresAt != nil {
			until = refreshClaims.ExpiresAt.Time
		}
		won, rerr := s.opts.TokenStore.RevokeIfNotRevoked(ctx, refreshClaims.ID, until)
		if rerr != nil {
			return nil, fmt.Errorf("claim rotated refresh token: %w", rerr)
		}
		if !won {
			return nil, errs.ErrAccessDenied
		}
	}
	newRefresh, err := s.createToken(user.UID, user.RoleKey, user.TenantID, tokenTypeRefresh, __refreshTokenTTL)
	if err != nil {
		return nil, err
	}
	// Issue the access token from the FRESH DB row — refreshClaims.Role
	// may be stale for up to the old refresh TTL after a role change.
	accessToken, err := s.createToken(user.UID, user.RoleKey, user.TenantID, tokenTypeAccess, ttl)
	if err != nil {
		return nil, err
	}
	return &pb.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefresh,
		Uid:          user.UID,
		Expires:      ttl,
	}, nil
}

// Logout revokes the presented token(s) via the TokenStore: the access
// token's jti is denylisted until its expiry, and — when the client
// supplies it — the paired refresh token too, so a stolen refresh
// token cannot outlive the logout.  At least one token must be
// present.  Unknown / expired tokens are no-ops so logout stays
// idempotent.
func (s *AuthService) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.AccessToken == "" && req.RefreshToken == "" {
		return nil, errs.Newf(errs.CodeInvalid, "access_token or refresh_token required")
	}
	var uid string
	if req.AccessToken != "" {
		u, err := s.revokeToken(ctx, req.AccessToken)
		if err != nil {
			return nil, err
		}
		uid = u
	}
	if req.RefreshToken != "" {
		if _, err := s.revokeToken(ctx, req.RefreshToken); err != nil {
			return nil, err
		}
	}
	return &pb.LogoutResponse{Uid: uid}, nil
}

// revokeToken parses one raw JWT and denylists its jti until the
// token's natural expiry.  Unparseable, expired, or legacy (jti-less)
// tokens are silent no-ops — they can no longer authenticate anyone,
// and logout must not fail because of them.  Returns the token's UID
// ("" when nothing could be parsed) for LogoutResponse echo.
func (s *AuthService) revokeToken(ctx context.Context, raw string) (string, error) {
	claims := &auth.Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, s.keyfunc())
	if err != nil || !token.Valid {
		return "", nil
	}
	if claims.ID == "" || s.opts.TokenStore == nil {
		return claims.UID, nil
	}
	until := time.Now().Add(time.Duration(s.ttl()) * time.Second)
	if claims.ExpiresAt != nil && claims.ExpiresAt.After(time.Now()) {
		until = claims.ExpiresAt.Time
	}
	if err := s.opts.TokenStore.Revoke(ctx, claims.ID, until); err != nil {
		return "", fmt.Errorf("revoke token: %w", err)
	}
	return claims.UID, nil
}
