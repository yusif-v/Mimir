package mcpserver

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures how the server is served.
type Options struct {
	HTTPAddr string // empty => stdio
	Token    string // bearer token for HTTP; see Task 7
}

// Serve runs the MCP server until the context is cancelled or the client
// disconnects.
func Serve(ctx context.Context, s *Server, opts Options) error {
	if opts.HTTPAddr == "" {
		return s.srv.Run(ctx, &mcp.StdioTransport{})
	}
	return serveHTTP(ctx, s, opts)
}

// normalizeHTTPAddr rewrites an empty host (e.g. ":7777") to explicit loopback
// so the address that gets bound matches the address the auth decision is made
// about. An empty host binds all interfaces, so leaving it as-is would let a
// tokenless ":7777" be classified as loopback while actually listening on
// 0.0.0.0 — a network-wide exposure with no auth.
func normalizeHTTPAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid --http address %q: %w", addr, err)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}

// requireLoopbackOrToken enforces that non-loopback HTTP addresses are only
// served with a bearer token configured. Loopback and localhost addresses are
// allowed tokenless. An empty host is treated as non-loopback (it binds all
// interfaces) and therefore requires a token; callers that want the loopback
// convenience should normalizeHTTPAddr first.
func requireLoopbackOrToken(addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --http address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	if token == "" {
		return fmt.Errorf("binding non-loopback address %q requires a token (--token or MIMIR_MCP_TOKEN)", addr)
	}
	return nil
}

// serveHTTP runs the MCP server over the Streamable HTTP transport.
func serveHTTP(ctx context.Context, s *Server, opts Options) error {
	addr, err := normalizeHTTPAddr(opts.HTTPAddr)
	if err != nil {
		return err
	}
	if err := requireLoopbackOrToken(addr, opts.Token); err != nil {
		return err
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.srv }, nil)
	var handler http.Handler = h
	if opts.Token != "" {
		handler = bearerAuth(h, opts.Token)
	}
	srv := &http.Server{Addr: addr, Handler: handler}
	go func() { <-ctx.Done(); _ = srv.Close() }()
	fmt.Fprintf(os.Stderr, "mimir mcp: serving Streamable HTTP on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// bearerAuth wraps next requiring a matching "Authorization: Bearer <token>"
// header, comparing in constant time.
func bearerAuth(next http.Handler, token string) http.Handler {
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
