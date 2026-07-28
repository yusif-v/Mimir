package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Options configures how the server is served.
type Options struct {
	HTTPAddr string // empty => stdio
	Token    string // bearer token for HTTP; see Task 7
}

// Serve runs the MCP server until the context is cancelled or the client
// disconnects. HTTP handling is added in Task 7.
func Serve(ctx context.Context, s *Server, opts Options) error {
	if opts.HTTPAddr == "" {
		return s.srv.Run(ctx, &mcp.StdioTransport{})
	}
	return serveHTTP(ctx, s, opts) // implemented in Task 7
}

// serveHTTP is a placeholder until Task 7 implements the Streamable HTTP
// transport.
func serveHTTP(ctx context.Context, s *Server, opts Options) error {
	return fmt.Errorf("http transport not yet implemented")
}
