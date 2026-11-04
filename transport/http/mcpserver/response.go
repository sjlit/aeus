package mcpserver

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ResponseMarshal converts a handler result into MCP content.
type ResponseMarshal func(ctx context.Context, res any) (content []sdk.Content, err error)

// DefaultResponseMarshal serialises res as a single JSON text block.
var DefaultResponseMarshal ResponseMarshal = func(_ context.Context, res any) ([]sdk.Content, error) {
	buf, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}
	return []sdk.Content{
		&sdk.TextContent{Text: string(buf)},
	}, nil
}
