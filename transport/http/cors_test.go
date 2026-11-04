package http

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	svr := New(
		WithAddress(":0"),
		WithDebug(true),
		WithCORS(CORSConfig{
			AllowOrigins: []string{"https://example.com"},
			AllowHeaders: []string{"X-Token"},
			MaxAge:       time.Hour,
		}),
	)
	ep := startFixTest(t, svr)

	req, _ := http.NewRequest(http.MethodOptions, ep+"/any", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "X-Token")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Allow-Origin = %q, want the request origin", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Fatalf("Allow-Methods = %q, want POST", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "X-Token" {
		t.Fatalf("Allow-Headers = %q, want X-Token", got)
	}
	if got := resp.Header.Get("Access-Control-Max-Age"); got != "3600" {
		t.Fatalf("Max-Age = %q, want 3600", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Allow-Credentials = %q, want empty", got)
	}
	if vary := strings.Join(resp.Header.Values("Vary"), ","); !strings.Contains(vary, "Origin") {
		t.Fatalf("Vary = %q, want Origin", vary)
	}
}

func TestCORS_DisallowedOriginGetsNoHeaders(t *testing.T) {
	svr := New(
		WithAddress(":0"),
		WithDebug(true),
		WithCORS(CORSConfig{AllowOrigins: []string{"https://example.com"}}),
	)
	ep := startFixTest(t, svr)

	req, _ := http.NewRequest(http.MethodOptions, ep+"/any", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
}

func TestCORS_Wildcard(t *testing.T) {
	svr := New(
		WithAddress(":0"),
		WithDebug(true),
		WithCORS(CORSConfig{AllowOrigins: []string{"*"}}),
	)
	ep := startFixTest(t, svr)

	req, _ := http.NewRequest(http.MethodGet, ep+"/any", nil)
	req.Header.Set("Origin", "https://anything.example")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Allow-Credentials = %q, want empty with wildcard", got)
	}
}

func TestCORS_CredentialsReflectsOrigin(t *testing.T) {
	svr := New(
		WithAddress(":0"),
		WithDebug(true),
		WithCORS(CORSConfig{
			AllowOrigins:     []string{"https://example.com"},
			AllowCredentials: true,
		}),
	)
	ep := startFixTest(t, svr)

	req, _ := http.NewRequest(http.MethodOptions, ep+"/any", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("Allow-Origin = %q, want reflected origin", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q, want true", got)
	}
}

func TestCORS_InvalidConfigPanics(t *testing.T) {
	tests := []struct {
		name string
		cfg  CORSConfig
	}{
		{name: "empty origins", cfg: CORSConfig{}},
		{name: "wildcard with credentials", cfg: CORSConfig{AllowOrigins: []string{"*"}, AllowCredentials: true}},
		{name: "origin with path", cfg: CORSConfig{AllowOrigins: []string{"https://example.com/api"}}},
		{name: "negative max age", cfg: CORSConfig{AllowOrigins: []string{"https://example.com"}, MaxAge: -time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("New(%s) did not panic", tt.name)
				}
			}()
			New(WithAddress(":0"), WithCORS(tt.cfg))
		})
	}
}
