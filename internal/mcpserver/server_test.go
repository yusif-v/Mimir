package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/shell"
)

func newTestServer(t *testing.T) (*mcp.ClientSession, *shell.App) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.CasesPath = t.TempDir()
	app := shell.NewApp(cfg)
	srv := NewServer(app)

	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := srv.MCP().Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs, app
}

func TestCasesListEmpty(t *testing.T) {
	cs, _ := newTestServer(t)
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "cases_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
}
