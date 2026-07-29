// Package main is the entry point for Mimir.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yusif-v/mimir/internal/config"
	"github.com/yusif-v/mimir/internal/mcpserver"
	"github.com/yusif-v/mimir/internal/shell"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			handleConfigCmd()
			return
		case "mcp":
			handleMCPCmd()
			return
		case "--version", "-v", "version":
			fmt.Printf("mimir %s\n", shell.Version)
			return
		}
	}

	cfg := config.Load()
	app := shell.NewApp(cfg)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func handleConfigCmd() {
	cfg := config.DefaultConfig()
	path := cfg.ConfigPath()

	if len(os.Args) > 2 {
		path = os.Args[2]
	}

	// Ensure parent directory exists
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating directory: %v\n", err)
			os.Exit(1)
		}
	}

	if err := config.WriteDefault(path); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Default config written to: %s\n", path)
	fmt.Printf("Edit it to customize Mimir's behavior.\n")
}

func handleMCPCmd() {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	httpAddr := fs.String("http", "", "serve Streamable HTTP on this address instead of stdio")
	token := fs.String("token", os.Getenv("MIMIR_MCP_TOKEN"), "bearer token for HTTP transport")
	_ = fs.Parse(os.Args[2:])

	cfg := config.Load()
	app := shell.NewApp(cfg)
	if err := mcpserver.Serve(context.Background(), mcpserver.NewServer(app), mcpserver.Options{
		HTTPAddr: *httpAddr,
		Token:    *token,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "mcp: %v\n", err)
		os.Exit(1)
	}
}
