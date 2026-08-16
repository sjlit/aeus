package http

import (
	"context"
	"encoding/json"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type (
	mcpContextKey struct{}

	McpResponseMarshal func(ctx context.Context, res any) (content []mcp.Content, err error)
)

var (
	mcpCtxKey = mcpContextKey{}
)

var (
	DefaultMcpResponseMarshal = func(ctx context.Context, res any) (content []mcp.Content, err error) {
		buf, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		return []mcp.Content{
			&mcp.TextContent{Text: string(buf)},
		}, nil
	}
)

// WithMcpRequestContext sets the CallToolRequest in the context.
func WithMcpRequestContext(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	return context.WithValue(ctx, mcpCtxKey, req)
}

// GetMcpRequestContext returns the CallToolRequest from the context.
func GetMcpRequestContext(ctx context.Context) *mcp.CallToolRequest {
	v := ctx.Value(mcpCtxKey)
	if v == nil {
		return nil
	}
	if req, ok := v.(*mcp.CallToolRequest); ok {
		return req
	}
	return nil
}
