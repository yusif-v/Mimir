package mcpserver

import (
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/shell"
)

// Server adapts Mimir's subsystems to the Model Context Protocol.
type Server struct {
	app *shell.App
	mu  sync.Mutex
	srv *mcp.Server
}

// NewServer builds an MCP server exposing Mimir's DFIR tools.
func NewServer(app *shell.App) *Server {
	s := &Server{
		app: app,
		srv: mcp.NewServer(&mcp.Implementation{Name: "mimir", Version: shell.Version}, nil),
	}
	s.registerCaseTools()
	s.registerTimelineTools()
	s.registerEvidenceTools()
	s.registerIOCTools()
	s.registerExecTools()
	s.registerReportTools()
	return s
}

// MCP returns the underlying protocol server.
func (s *Server) MCP() *mcp.Server { return s.srv }

// withCase opens the named case under the server mutex and runs fn against it.
func (s *Server) withCase(name string, fn func(*cases.Case) (any, error)) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.app.Cases.Load(name)
	if err != nil {
		return nil, err
	}
	return fn(c)
}
