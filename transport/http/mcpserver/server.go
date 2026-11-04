package mcpserver

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	ghttp "github.com/sjlit/aeus/transport/http"
)

// Server owns an MCP SDK server and the HTTP wiring needed to expose it.
type Server struct {
	sdk *sdk.Server
	cfg Config
}

// New builds an MCP server from cfg.
func New(cfg Config) *Server {
	return newServer(cfg, nil)
}

// NewWithServer wraps an already configured SDK server. Tool, prompt and
// resource registration stays with the caller.
func NewWithServer(cfg Config, server *sdk.Server) *Server {
	if server == nil {
		panic("mcpserver: NewWithServer requires a non-nil server")
	}
	return newServer(cfg, server)
}

func newServer(cfg Config, server *sdk.Server) *Server {
	if server == nil {
		impl := &sdk.Implementation{Name: cfg.Name, Version: cfg.Version}
		if impl.Name == "" {
			impl.Name = "aeus"
		}
		if impl.Version == "" {
			impl.Version = "0.0.0"
		}
		var opts *sdk.ServerOptions
		if cfg.Instructions != "" {
			opts = &sdk.ServerOptions{Instructions: cfg.Instructions}
		}
		server = sdk.NewServer(impl, opts)
	}
	return &Server{sdk: server, cfg: cfg}
}

// SDK returns the underlying MCP SDK server for tool registration.
func (s *Server) SDK() *sdk.Server {
	return s.sdk
}

// Attach registers the streamable HTTP handler on hs. It must be called
// before hs.Start.
func (s *Server) Attach(hs *ghttp.Server) {
	if hs == nil {
		panic("mcpserver: Attach requires a non-nil transport/http server")
	}
	streamableOpts := &sdk.StreamableHTTPOptions{
		Stateless:      true,
		SessionTimeout: s.cfg.SessionTimeout,
	}
	if s.cfg.Streamable != nil {
		s.cfg.Streamable(streamableOpts)
	}

	var handler http.Handler = sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return s.sdk },
		streamableOpts,
	)
	verifier := s.cfg.TokenVerifier
	if verifier == nil && s.cfg.Authorization != "" {
		verifier = defaultTokenVerifier(s.cfg.Authorization)
	}
	if verifier != nil {
		handler = auth.RequireBearerToken(auth.TokenVerifier(verifier), &auth.RequireBearerTokenOptions{
			Scopes: []string{"access"},
		})(handler)
	}

	path := s.cfg.Path
	if path == "" {
		path = "/mcp"
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		hs.Handle(method, path, handler.ServeHTTP)
	}
}

// defaultTokenVerifier compares the bearer token against expected using a
// constant-time comparison to prevent timing side channels. Verified tokens
// are granted the "access" scope for two hours.
func defaultTokenVerifier(expected string) TokenVerifier {
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
