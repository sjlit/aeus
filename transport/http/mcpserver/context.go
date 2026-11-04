package mcpserver

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type contextKey struct{}

// WithRequestContext stores the CallToolRequest in ctx.
func WithRequestContext(ctx context.Context, req *sdk.CallToolRequest) context.Context {
	return context.WithValue(ctx, contextKey{}, req)
}

// RequestFromContext returns the CallToolRequest stored in ctx, if any.
func RequestFromContext(ctx context.Context) *sdk.CallToolRequest {
	req, _ := ctx.Value(contextKey{}).(*sdk.CallToolRequest)
	return req
}
