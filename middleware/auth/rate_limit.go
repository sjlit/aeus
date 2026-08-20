package auth

import (
	"context"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/middleware"
	"github.com/sjlit/aeus/pkg/errs"
)

// Limiter is the storage backend the RateLimit middleware consults on
// every request.
//
// Allow is called once per resolved key (per the path / templates
// below); when any key returns allowed=false the request is rejected
// with 4010 TooManyAttempts and the returned retryAfter is written to
// the Retry-After response header so well-behaved clients can back
// off without a probe loop.
//
// Implementations MUST be safe for concurrent use and MUST NOT block
// on I/O unboundedly (the middleware sits ahead of every request to
// the protected path prefix). The default in-memory implementation
// (NewMemoryLimiter) is a token-bucket per key with 5-minute idle GC;
// a Redis-backed implementation will follow the same Allow contract.
//
// Returning a non-nil error surfaces a 503-style failure (CodeInternal
// at the middleware layer) rather than silently letting traffic
// through — fail-closed is the only safe choice on storage failure.
type Limiter interface {
	Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error)
}

// RateLimitOpts configures one RateLimit middleware instance. A single
// RateLimit call protects a path prefix; stack two calls if /auth/
// login needs a stricter rule than /auth/refresh-token.
type RateLimitOpts struct {
	// Path is the request-path prefix the middleware protects. The
	// same segment-aware syntax as JWT.WithAllow applies (exact,
	// "*", "<prefix>*" anchored at a segment boundary). Requests
	// outside this prefix pass straight through.
	Path string

	// Keys is the set of key templates evaluated per request. Each
	// template may embed placeholders enclosed in braces; the
	// middleware substitutes them from request metadata and (if
	// present) the parsed JWT claims. A missing placeholder
	// resolves to the empty string, so "user:{user}" without a
	// token degrades to "user:" and still participates in the
	// decision (one global bucket for anonymous traffic).
	//
	// Templates support:
	//
	//   {ip}      → metadata.RequestClientIP
	//   {user}    → claims.UID (jwt RegisteredClaims "sub"
	//                fallback) — empty before JWT middleware runs
	//   {tenant}  → claims TenantID — empty before JWT runs
	//   {path}    → metadata.RequestPath
	//
	// When more than one template is supplied the middleware
	// short-circuits on the FIRST Allow that returns allowed=false
	// (logical AND over independent buckets). This means a request
	// hitting both an exhausted IP bucket and an exhausted user
	// bucket is rejected as soon as the first is detected.
	Keys []string

	// Capacity is the token-bucket capacity (burst size).
	// Must be > 0.
	Capacity int

	// Refill is the steady-state refill rate, tokens per second.
	// Must be > 0. A typical "5/minute" rule is Capacity=5,
	// Refill=1.0/60.
	Refill float64

	// Limiter overrides the in-memory default. When nil, the
	// middleware builds a NewMemoryLimiter keyed by
	// (Path, Capacity, Refill) — see package-level docs for the
	// trade-off (single-process only).
	Limiter Limiter
}

// RateLimit returns a middleware that protects the configured path
// prefix with a token-bucket rate limit keyed on the supplied
// templates.
//
// Order in the chain: place RateLimit BEFORE JWT when the protected
// path sits on the JWT allowlist (e.g. /auth/login) — failing the
// limit before any DB or signature work is the whole point. Place it
// AFTER JWT when the limit should apply per authenticated subject
// (the user placeholder resolves to the caller's UID).
//
// On rejection the middleware returns *errs.Error with code 4010
// TooManyAttempts (HTTP 429). The Retry-After header is published via
// the metadata response writer when one is wired in; see
// transport/http for the integration.
func RateLimit(opts RateLimitOpts) middleware.Middleware {
	if opts.Capacity <= 0 {
		panic("middleware/auth: RateLimit requires Capacity > 0")
	}
	if opts.Refill <= 0 {
		panic("middleware/auth: RateLimit requires Refill > 0")
	}
	if opts.Path == "" {
		panic("middleware/auth: RateLimit requires a non-empty Path")
	}
	if len(opts.Keys) == 0 {
		panic("middleware/auth: RateLimit requires at least one key template")
	}
	lim := opts.Limiter
	if lim == nil {
		lim = NewMemoryLimiter(opts.Capacity, opts.Refill)
	}
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context) error {
			path, _ := metadata.Get(ctx, metadata.RequestPath)
			if !isAllowed(path, []string{opts.Path}) {
				return next(ctx)
			}
			for _, tmpl := range opts.Keys {
				key := expandKey(ctx, tmpl)
				allowed, retryAfter, err := lim.Allow(ctx, key)
				if err != nil {
					// Fail-closed: a misbehaving limiter MUST NOT let
					// traffic through unchecked.
					return errs.Wrap(errs.CodeInternal, err)
				}
				if !allowed {
					if retryAfter > 0 {
						publishRetryAfter(ctx, retryAfter)
					}
					return errs.Newf(
						errs.CodeTooManyAttempts,
						"too many attempts, retry after %s", retryAfter,
					)
				}
			}
			return next(ctx)
		}
	}
}

// expandKey substitutes {ip}, {user}, {tenant}, {path} placeholders
// in tmpl. Unknown placeholders are left literal so a typo is
// visible (the bucket key becomes the template minus the
// recognised slots, which keeps the limiter honest). Empty values
// are still substituted in (so "user:" lands in the same bucket as
// every other anonymous request — typically the desired behaviour
// for unauthenticated traffic).
func expandKey(ctx context.Context, tmpl string) string {
	var ip, user, tenant, path string
	ip, _ = metadata.Get(ctx, metadata.RequestClientIP)
	path, _ = metadata.Get(ctx, metadata.RequestPath)
	if claims, ok := FromContext(ctx); ok && claims != nil {
		// jwt.MapClaims IS map[string]any and implements jwt.Claims
		// via the helper methods (GetExpirationTime, etc.). admin's
		// own *auth.Claims is a struct embedding RegisteredClaims
		// — for that case admin's claims package's own
		// ClaimsFromContext helpers are the right entry point, but
		// the RateLimit middleware is transport-layer and shouldn't
		// depend on admin. We accept "best effort": when the parsed
		// claims aren't a MapClaims (e.g. *auth.Claims), {user}
		// resolves to "" and the request falls into the anonymous
		// bucket. Place RateLimit AFTER JWT if you need per-user
		// limiting — admin can wire it that way.
		if mc, ok := claims.(jwt.MapClaims); ok {
			if v, ok := mc["uid"].(string); ok && v != "" {
				user = v
			} else if v, ok := mc["sub"].(string); ok && v != "" {
				user = v
			}
			if v, ok := mc["tid"].(string); ok {
				tenant = v
			}
		}
	}
	out := tmpl
	out = strings.ReplaceAll(out, "{ip}", ip)
	out = strings.ReplaceAll(out, "{user}", user)
	out = strings.ReplaceAll(out, "{tenant}", tenant)
	out = strings.ReplaceAll(out, "{path}", path)
	return out
}

// publishRetryAfter stamps the response Retry-After header when a
// TeeWriter is in ctx (transport/http sets one up). On other
// transports the call is a no-op — Retry-After is an HTTP concept.
func publishRetryAfter(ctx context.Context, retryAfter time.Duration) {
	seconds := max(int64(retryAfter.Round(time.Second).Seconds()), 1)
	metadata.Set(ctx, "Retry-After", itoa(seconds))
}

// itoa is a tiny strconv.Itoa equivalent kept local to avoid pulling
// strconv into a hot path that callers may invoke per request.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}