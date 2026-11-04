package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var defaultCORSMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
}

// validateCORS rejects configurations that would either silently disable
// CORS or produce responses browsers refuse to honour.
func validateCORS(cfg CORSConfig) error {
	if len(cfg.AllowOrigins) == 0 {
		return errors.New("AllowOrigins is required")
	}
	wildcard := false
	for _, origin := range cfg.AllowOrigins {
		if origin == "*" {
			wildcard = true
			continue
		}
		u, err := url.Parse(origin)
		if err != nil ||
			(!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) ||
			u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid origin %q (want scheme://host[:port])", origin)
		}
	}
	if wildcard && cfg.AllowCredentials {
		return errors.New(`AllowOrigins "*" cannot be combined with AllowCredentials`)
	}
	if cfg.MaxAge < 0 {
		return errors.New("MaxAge must not be negative")
	}
	for _, method := range cfg.AllowMethods {
		if method == "" || method != strings.ToUpper(method) {
			return fmt.Errorf("invalid method %q (want an uppercase HTTP method)", method)
		}
	}
	return nil
}

func originAllowed(origins []string, origin string) bool {
	for _, allowed := range origins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

// CORSInterceptor returns the middleware installed by WithCORS. It reflects
// only allow-listed origins and handles preflight requests without ever
// pairing a wildcard origin with credentials.
func (s *Server) CORSInterceptor() gin.HandlerFunc {
	cfg := s.opts.cors
	if cfg == nil {
		cfg = &CORSConfig{}
	}
	methods := cfg.AllowMethods
	if len(methods) == 0 {
		methods = defaultCORSMethods
	}
	methodsHeader := strings.Join(methods, ", ")
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")
	wildcard := false
	for _, origin := range cfg.AllowOrigins {
		if origin == "*" {
			wildcard = true
			break
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		header := c.Writer.Header()
		// Always vary on Origin so shared caches cannot serve a response
		// carrying CORS headers to a different origin.
		header.Add("Vary", "Origin")
		if !originAllowed(cfg.AllowOrigins, origin) {
			c.Next()
			return
		}

		switch {
		case cfg.AllowCredentials:
			header.Set("Access-Control-Allow-Origin", origin)
			header.Set("Access-Control-Allow-Credentials", "true")
		case wildcard:
			header.Set("Access-Control-Allow-Origin", "*")
		default:
			header.Set("Access-Control-Allow-Origin", origin)
		}
		if exposeHeaders != "" {
			header.Set("Access-Control-Expose-Headers", exposeHeaders)
		}
		if c.Request.Method != http.MethodOptions {
			c.Next()
			return
		}

		header.Add("Vary", "Access-Control-Request-Method")
		header.Add("Vary", "Access-Control-Request-Headers")
		header.Set("Access-Control-Allow-Methods", methodsHeader)
		if allowHeaders != "" {
			header.Set("Access-Control-Allow-Headers", allowHeaders)
		} else if requested := c.GetHeader("Access-Control-Request-Headers"); requested != "" {
			header.Set("Access-Control-Allow-Headers", requested)
		}
		if cfg.MaxAge > 0 {
			header.Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge/time.Second)))
		}
		c.AbortWithStatus(http.StatusNoContent)
	}
}
