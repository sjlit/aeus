package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	ghttp "github.com/sjlit/aeus/transport/http"
)

func start(t *testing.T, cfg Config) string {
	t.Helper()
	svr := ghttp.New(ghttp.WithAddress(":0"))
	New(cfg).Attach(svr)
	ts := httptest.NewServer(svr.Engine())
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestDefaultTokenVerifier_WrongTokenUnwrapsToInvalid(t *testing.T) {
	verifier := defaultTokenVerifier("secret")

	info, err := verifier(context.Background(), "wrong", nil)
	if info != nil {
		t.Fatalf("expected nil TokenInfo on bad token, got %+v", info)
	}
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("err = %v, want errors.Is(_, auth.ErrInvalidToken) == true", err)
	}
}

func TestDefaultTokenVerifier_CorrectTokenGrantsAccess(t *testing.T) {
	verifier := defaultTokenVerifier("secret")

	before := time.Now()
	info, err := verifier(context.Background(), "secret", nil)
	after := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("TokenInfo is nil")
	}
	if len(info.Scopes) != 1 || info.Scopes[0] != "access" {
		t.Errorf("Scopes = %v, want [access]", info.Scopes)
	}
	// Expiration must fall inside the [before+2h, after+2h] window.
	wantMin := before.Add(time.Hour * 2)
	wantMax := after.Add(time.Hour * 2)
	if info.Expiration.Before(wantMin) || info.Expiration.After(wantMax) {
		t.Errorf("Expiration = %v, want within [%v, %v]", info.Expiration, wantMin, wantMax)
	}
}

func TestNewServer_AppliesDefaults(t *testing.T) {
	srv := New(Config{})
	if srv.SDK() == nil {
		t.Fatal("New returned a nil SDK server")
	}
}

func TestNewWithServer_InjectsPrebuilt(t *testing.T) {
	prebuilt := sdk.NewServer(&sdk.Implementation{Name: "prebuilt"}, nil)
	srv := NewWithServer(Config{}, prebuilt)
	if got := srv.SDK(); got != prebuilt {
		t.Errorf("SDK() did not return the injected server")
	}
}

// TestServer_MountsAtDefaultPath verifies the default /mcp route is
// registered when Path is left empty.
func TestServer_MountsAtDefaultPath(t *testing.T) {
	base := start(t, Config{})

	resp, err := http.Get(base + "/mcp")
	if err != nil {
		t.Fatalf("GET /mcp: %v", err)
	}
	defer resp.Body.Close()
	// Any non-405 status proves the route is mounted (gin's NoRoute returns
	// 404 with the JSON envelope, the MCP handler returns its own codes).
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Errorf("/mcp returned 405; expected the MCP handler to be registered")
	}
}

func TestServer_MountsAtCustomPath(t *testing.T) {
	base := start(t, Config{Path: "/custom-mcp"})

	if resp, err := http.Get(base + "/mcp"); err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET /mcp on custom-path server = %d, want 404", resp.StatusCode)
		}
	}
	resp, err := http.Get(base + "/custom-mcp")
	if err != nil {
		t.Fatalf("GET /custom-mcp: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Errorf("/custom-mcp returned 405; expected the MCP handler to be registered")
	}
}

// TestServer_RejectsMissingToken401 covers the auth middleware branch:
// no Authorization header → 401, not 500.
func TestServer_RejectsMissingToken401(t *testing.T) {
	base := start(t, Config{Authorization: "secret"})

	resp, err := http.Post(base+"/mcp", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("missing-token status = %d (%s), want 401", resp.StatusCode, body)
	}
}

// TestServer_RejectsBadToken401 is the regression guard for the
// defaultTokenVerifier fix: returning auth.ErrInvalidToken makes the
// upstream middleware map to 401. A wrong error type would surface as 500.
func TestServer_RejectsBadToken401(t *testing.T) {
	base := start(t, Config{Authorization: "secret"})

	req, _ := http.NewRequest(http.MethodPost, base+"/mcp", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("bad-token status = %d (%s), want 401", resp.StatusCode, body)
	}
}

func TestServer_CustomTokenVerifierIsInvoked(t *testing.T) {
	called := false
	verifier := func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		called = true
		if token != "custom" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{
			Scopes:     []string{"access"},
			Expiration: time.Now().Add(time.Hour),
		}, nil
	}
	base := start(t, Config{TokenVerifier: verifier})

	// Wrong token → 401, confirms the verifier ran.
	req, _ := http.NewRequest(http.MethodPost, base+"/mcp", bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer nope")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("custom-verifier bad-token status = %d, want 401", resp.StatusCode)
	}
	if !called {
		t.Error("custom verifier was not invoked")
	}
}

func TestServer_NoAuthWhenUnconfigured(t *testing.T) {
	// No Authorization AND no custom verifier → no RequireBearerToken
	// middleware; the request reaches the MCP handler directly. An empty
	// POST should NOT come back as 401.
	base := start(t, Config{})

	resp, err := http.Post(base+"/mcp", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("status = 401, want non-401 when auth is unconfigured")
	}
}

func TestDefaultResponseMarshal(t *testing.T) {
	content, err := DefaultResponseMarshal(context.Background(), map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("default marshal error: %v", err)
	}
	if len(content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(content))
	}
	tc, ok := content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *sdk.TextContent", content[0])
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(tc.Text), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("payload = %v, want {k:v}", got)
	}
}

func TestRequestContextRoundTrip(t *testing.T) {
	want := &sdk.CallToolRequest{}
	ctx := WithRequestContext(context.Background(), want)
	if got := RequestFromContext(ctx); got != want {
		t.Fatalf("RequestFromContext = %p, want %p", got, want)
	}
	if got := RequestFromContext(context.Background()); got != nil {
		t.Fatalf("RequestFromContext(empty) = %v, want nil", got)
	}
}
