// Package mcpserver adapts Mimir's subsystems to the Model Context Protocol
// (MCP), exposing DFIR case/tool/evidence operations as MCP tools over
// stdio or Streamable HTTP.
package mcpserver

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// textResult wraps a human-readable summary (structured Out is returned
// separately by each typed handler).
func textResult(summary string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: summary}},
	}
}

// errResult reports a domain failure to the client as a tool error.
func errResult(format string, a ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, a...)}},
	}
}
