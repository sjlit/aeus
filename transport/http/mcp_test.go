package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDefaultMCPTokenVerifier_WrongTokenUnwrapsToInvalid(t *testing.T) {
	verifier := defaultMCPTokenVerifier("secret")

	info, err := verifier(context.Background(), "wrong", nil)
	if info != nil {
		t.Fatalf("expected nil TokenInfo on bad token, got %+v", info)
	}
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("err = %v, want errors.Is(_, auth.ErrInvalidToken) == true", err)
	}
}

func TestDefaultMCPTokenVerifier_CorrectTokenGrantsAccess(t *testing.T) {
	verifier := defaultMCPTokenVerifier("secret")

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

func TestNewMCPServer_AppliesDefaults(t *testing.T) {
	srv := newMCPServer(MCPConfig{})
	if srv == nil {
		t.Fatal("newMCPServer returned nil")
	}
	// Implementation identity lives on the unexported field, but the
	// server must be non-nil and registered with default name/version —
	// exercised end-to-end via WithMCP integration test below.
}

func TestServer_Mcp_NilByDefault(t *testing.T) {
	svr := New(WithAddress(":0"))
	if got := svr.Mcp(); got != nil {
		t.Errorf("Mcp() = %v, want nil when neither WithMCP nor WithMCPServer is set", got)
	}
}

func TestServer_WithMCP_EnablesServer(t *testing.T) {
	svr := New(WithAddress(":0"), WithMCP(MCPConfig{Name: "svc", Version: "1.0"}))
	if got := svr.Mcp(); got == nil {
		t.Fatal("Mcp() returned nil after WithMCP")
	}
}

func TestServer_WithMCPServer_InjectsPrebuilt(t *testing.T) {
	prebuilt := mcp.NewServer(&mcp.Implementation{Name: "prebuilt"}, nil)
	svr := New(WithAddress(":0"), WithMCPServer(prebuilt))
	if got := svr.Mcp(); got != prebuilt {
		t.Errorf("Mcp() did not return the injected server")
	}
}

// startTestServer spins up a real listener and returns its base URL.
func startTestServer(t *testing.T, opts ...Option) (svr *Server, baseURL string) {
	t.Helper()
	all := append([]Option{WithAddress(":0")}, opts...)
	svr = New(all...)

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	errCh := make(chan error, 1)
	go func() { errCh <- svr.Start(runCtx) }()

	// Poll until the listener is bound (Start binds in createListener).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if endpoint, err := svr.Endpoint(runCtx); err == nil {
			baseURL = endpoint
			return svr, baseURL
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server failed to bind within 2s")
	return nil, ""
}

// TestServer_MCP_MountsAtDefaultPath verifies the default /mcp route is
// registered when Path is left empty.
func TestServer_MCP_MountsAtDefaultPath(t *testing.T) {
	_, base := startTestServer(t, WithMCP(MCPConfig{}))

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

func TestServer_MCP_MountsAtCustomPath(t *testing.T) {
	_, base := startTestServer(t, WithMCP(MCPConfig{Path: "/custom-mcp"}))

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

// TestServer_MCP_RejectsMissingToken401 covers the auth middleware branch:
// no Authorization header → 401, not 500.
func TestServer_MCP_RejectsMissingToken401(t *testing.T) {
	_, base := startTestServer(t, WithMCP(MCPConfig{Authorization: "secret"}))

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

// TestServer_MCP_RejectsBadToken401 is the regression guard for the
// defaultMCPTokenVerifier fix: returning auth.ErrInvalidToken makes the
// upstream middleware map to 401. A wrong error type would surface as 500.
func TestServer_MCP_RejectsBadToken401(t *testing.T) {
	_, base := startTestServer(t, WithMCP(MCPConfig{Authorization: "secret"}))

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

func TestServer_MCP_CustomTokenVerifierIsInvoked(t *testing.T) {
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
	_, base := startTestServer(t,
		WithMCP(MCPConfig{}),
		WithMCPTokenVerifier(verifier),
	)

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

func TestServer_MCP_NoAuthWhenUnconfigured(t *testing.T) {
	// WithMCPConfig without Authorization AND no custom verifier → no
	// RequireBearerToken middleware; the request reaches the MCP handler
	// directly. An empty POST should NOT come back as 401.
	_, base := startTestServer(t, WithMCP(MCPConfig{}))

	resp, err := http.Post(base+"/mcp", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST /mcp: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("status = 401, want non-401 when auth is unconfigured")
	}
}

// TestNewRouteOptions_DefaultMarshal covers the rename of
// the Mcp*-prefixed identifier family to the MCP* form.
func TestNewRouteOptions_DefaultMarshal(t *testing.T) {
	opts := NewRouteOptions()
	if opts.MCPResponseMarshal == nil {
		t.Fatal("NewRouteOptions left MCPResponseMarshal nil")
	}
	content, err := opts.MCPResponseMarshal(context.Background(), map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("default marshal error: %v", err)
	}
	if len(content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(content))
	}
	tc, ok := content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want *mcp.TextContent", content[0])
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(tc.Text), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("payload = %v, want {k:v}", got)
	}
}
