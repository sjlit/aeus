// Package mcpserver provides the optional MCP (Model Context Protocol)
// streamable HTTP server for transport/http.
//
// Keeping it in a separate package means applications that only use the
// HTTP transport do not import the MCP SDK or download it during a build.
package mcpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config configures the embedded MCP server.
type Config struct {
	Name         string // default "aeus"
	Version      string // default "0.0.0"
	Path         string // mount path, default "/mcp"
	Instructions string // mapped to sdk.ServerOptions.Instructions

	SessionTimeout time.Duration // idle session timeout, 0 = never close
	Authorization  string        // static bearer token; pairs with the default TokenVerifier

	// TokenVerifier replaces the default bearer-token check. When unset,
	// verification falls back to a constant-time compare against
	// Authorization (no auth if that is also empty).
	TokenVerifier TokenVerifier

	// Streamable, when set, is called after the defaults are applied,
	// allowing fine-tuning of the underlying StreamableHTTPOptions
	// (Stateless, JSONResponse, EventStore, ...). Defaults:
	// Stateless=true, JSONResponse=false.
	Streamable func(*sdk.StreamableHTTPOptions)
}

// TokenVerifier validates a bearer token presented to the MCP endpoint.
type TokenVerifier func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error)
