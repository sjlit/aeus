package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type (
	mcpContextKey struct{}

	MCPResponseMarshal func(ctx context.Context, res any) (content []mcp.Content, err error)
)

var (
	mcpCtxKey = mcpContextKey{}
)

var (
	DefaultMCPResponseMarshal = func(ctx context.Context, res any) (content []mcp.Content, err error) {
		buf, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return []mcp.Content{
			&mcp.TextContent{Text: string(buf)},
		}, nil
	}
)

// WithMCPRequestContext sets the CallToolRequest in the context.
func WithMCPRequestContext(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	return context.WithValue(ctx, mcpCtxKey, req)
}

// GetMCPRequestContext returns the CallToolRequest from the context.
func GetMCPRequestContext(ctx context.Context) *mcp.CallToolRequest {
	v := ctx.Value(mcpCtxKey)
	if v == nil {
		return nil
	}
	if req, ok := v.(*mcp.CallToolRequest); ok {
		return req
	}
	return nil
}

// newMCPServer builds the embedded MCP server from MCPConfig, applying
// defaults for the implementation identity and mapping Instructions.
func newMCPServer(cfg MCPConfig) *mcp.Server {
	impl := &mcp.Implementation{
		Name:    cfg.Name,
		Version: cfg.Version,
	}
	if impl.Name == "" {
		impl.Name = "aeus"
	}
	if impl.Version == "" {
		impl.Version = "0.0.0"
	}
	var opts *mcp.ServerOptions
	if cfg.Instructions != "" {
		opts = &mcp.ServerOptions{Instructions: cfg.Instructions}
	}
	return mcp.NewServer(impl, opts)
}

// defaultMCPTokenVerifier returns a TokenVerifier that compares the bearer
// token against expected using a constant-time comparison to prevent timing
// side channels. Verified tokens are granted the "access" scope for two hours.
func defaultMCPTokenVerifier(expected string) TokenVerifier {
	return func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		// Returning auth.ErrInvalidToken (not an arbitrary error) lets the
		// upstream RequireBearerToken middleware map the failure to 401
		// rather than 500 — it dispatches on errors.Is(err, ErrInvalidToken).
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{
			Scopes:     []string{"access"},
			Expiration: time.Now().Add(time.Hour * 2),
		}, nil
	}
}
