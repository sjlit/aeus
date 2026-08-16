package auth

import (
	"context"
	"reflect"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/sjlit/aeus/metadata"
	"github.com/sjlit/aeus/middleware"
	"github.com/sjlit/aeus/pkg/errs"
)

type authKey struct{}

const (

	// bearerWord the bearer key word for authorization
	bearerWord string = "Bearer"

	// bearerFormat authorization token format
	bearerFormat string = "Bearer %s"

	// authorizationKey holds the key used to store the JWT Token in the request tokenHeader.
	authorizationKey string = "Authorization"

	// reason holds the error reason.
	reason string = "UNAUTHORIZED"
)

type Option func(*options)

type PermissionCheckerFunc func(ctx context.Context, claims jwt.Claims) (err error)

type Validate interface {
	Validate(ctx context.Context, token string) error
}

// Parser is a jwt parser
type options struct {
	allows            []string
	claims            reflect.Type
	validate          Validate
	permissionChecker PermissionCheckerFunc
}

// WithAllow with allow path
func WithAllow(paths ...string) Option {
	return func(o *options) {
		if o.allows == nil {
			o.allows = make([]string, 0, 16)
		}
		for _, s := range paths {
			s = strings.TrimSpace(s)
			if len(s) == 0 {
				continue
			}
			o.allows = append(o.allows, s)
		}
	}
}

func WithClaims(claims any) Option {
	return func(o *options) {
		if tv, ok := claims.(reflect.Type); ok {
			o.claims = tv
		} else {
			o.claims = reflect.TypeOf(claims)
			if o.claims.Kind() == reflect.Ptr {
				o.claims = o.claims.Elem()
			}
		}
	}
}

func WithPermissionChecker(fn PermissionCheckerFunc) Option {
	return func(o *options) {
		o.permissionChecker = fn
	}
}

func WithValidate(fn Validate) Option {
	return func(o *options) {
		o.validate = fn
	}
}

// isAllowed checks if the path matches any allowlist pattern. The
// pattern syntax is:
//
//   - "*"            : match every path;
//   - "<exact>"      : match the path verbatim;
//   - "<prefix>*"    : match paths that share the prefix at a SEGMENT
//     boundary. "<prefix>/*" matches "/foo/" and "/foo/bar" but not
//     "/foo-rogue", so an operator typing "/api/admin*" cannot
//     accidentally expose "/api/admin-rogue/bar".
func isAllowed(uripath string, allows []string) bool {
	for _, pattern := range allows {
		n := len(pattern)
		if pattern == uripath {
			return true
		}
		if pattern == "*" {
			return true
		}
		if n > 1 && pattern[n-1] == '*' {
			prefix := pattern[:n-1]
			if !strings.HasPrefix(uripath, prefix) {
				continue
			}
			// Anchor at a segment boundary: the prefix already ends in
			// '/' or the next char in uripath is '/' (or uripath equals
			// the prefix exactly).
			if prefix[len(prefix)-1] == '/' {
				return true
			}
			if len(uripath) > len(prefix) && uripath[len(prefix)] == '/' {
				return true
			}
		}
	}
	return false
}

// JWT auth middleware
func JWT(keyFunc jwt.Keyfunc, cbs ...Option) middleware.Middleware {
	opts := options{}
	for _, cb := range cbs {
		cb(&opts)
	}
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context) (err error) {
			md := metadata.FromContext(ctx)
			if len(opts.allows) > 0 {
				requestPath, ok := md.Get(metadata.RequestPath)
				if ok {
					if isAllowed(requestPath, opts.allows) {
						return next(ctx)
					}
				}
			}
			token, ok := md.Get(authorizationKey)
			if !ok {
				return errs.ErrAccessDenied
			}
			token, _ = strings.CutPrefix(token, bearerWord)
			token = strings.TrimSpace(token)

			if opts.validate != nil {
				if err = opts.validate.Validate(ctx, token); err != nil {
					return err
				}
			}

			var (
				ti *jwt.Token
			)
			if opts.claims != nil {
				if claims, ok := reflect.New(opts.claims).Interface().(jwt.Claims); ok {
					ti, err = jwt.ParseWithClaims(token, claims, keyFunc)
				}
			}
			if ti == nil {
				ti, err = jwt.Parse(token, keyFunc)
			}
			if err != nil {
				if errs.Is(err, jwt.ErrTokenMalformed) || errs.Is(err, jwt.ErrTokenUnverifiable) {
					return errs.ErrAccessDenied
				}
				if errs.Is(err, jwt.ErrTokenNotValidYet) || errs.Is(err, jwt.ErrTokenExpired) {
					return errs.ErrTokenExpired
				}
				return errs.ErrPermissionDenied
			}
			if !ti.Valid {
				return errs.ErrPermissionDenied
			}
			if opts.permissionChecker != nil {
				if err = opts.permissionChecker(ctx, ti.Claims); err != nil {
					return err
				}
			}
			ctx = NewContext(ctx, ti.Claims)
			return next(ctx)
		}
	}
}

// NewContext put auth info into context
func NewContext(ctx context.Context, info jwt.Claims) context.Context {
	return context.WithValue(ctx, authKey{}, info)
}

// FromContext extract auth info from context
func FromContext(ctx context.Context) (token jwt.Claims, ok bool) {
	token, ok = ctx.Value(authKey{}).(jwt.Claims)
	return
}
