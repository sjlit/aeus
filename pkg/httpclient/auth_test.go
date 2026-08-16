package httpclient

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBasicAuth_Token(t *testing.T) {
	auth := &BasicAuth{Username: "user", Password: "pass"}
	got := auth.Token()
	want := "Basic dXNlcjpwYXNz"
	if got != want {
		t.Errorf("Token() = %q, want %q", got, want)
	}
}

func TestBearerAuth_Token(t *testing.T) {
	auth := &BearerAuth{AccessToken: "abc123"}
	got := auth.Token()
	want := "Bearer abc123"
	if got != want {
		t.Errorf("Token() = %q, want %q", got, want)
	}
}

func TestBasicAuth_Token_EmptyCredentials(t *testing.T) {
	auth := &BasicAuth{Username: "", Password: ""}
	got := auth.Token()
	if !strings.HasPrefix(got, "Basic ") {
		t.Errorf("Token() should have Basic prefix, got %q", got)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, "Basic "))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if string(decoded) != ":" {
		t.Errorf("decoded = %q, want \":\"", string(decoded))
	}
}

func TestBasicAuth_Token_SpecialChars(t *testing.T) {
	auth := &BasicAuth{Username: "user", Password: "p:a:s:s"}
	got := auth.Token()
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, "Basic "))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if string(decoded) != "user:p:a:s:s" {
		t.Errorf("decoded = %q, want \"user:p:a:s:s\"", string(decoded))
	}
}

func TestBasicAuth_Token_RoundTrip(t *testing.T) {
	auth := &BasicAuth{Username: "alice", Password: "wonderland"}
	token := auth.Token()
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(token, "Basic "))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "alice" || parts[1] != "wonderland" {
		t.Errorf("roundtrip failed: got %q", string(decoded))
	}
}
