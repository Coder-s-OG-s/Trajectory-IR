package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/console"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
	trajirmcp "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/mcp"
)

// cmdConsole serves the dashboard from the same data dir hooks write to.
func cmdConsole(args []string, c cliIO) int {
	fs := flag.NewFlagSet("console", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	addr := fs.String("addr", envOr(c, "TRAJIR_CONSOLE_ADDR", "127.0.0.1:8787"), "listen address")
	data := fs.String("data", hosthook.DefaultConsoleDir(c.getenv), "dashboard data directory")
	token := fs.String("token", c.getenv("TRAJIR_CONSOLE_TOKEN"), "optional Bearer token for /v1/*")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := os.MkdirAll(*data, 0o755); err != nil {
		fmt.Fprintf(c.stderr, "trajir console: %v\n", err)
		return 1
	}
	store, err := console.OpenStore(*data)
	if err != nil {
		fmt.Fprintf(c.stderr, "trajir console: %v\n", err)
		return 1
	}
	srv := console.NewServer(store, *token)
	fmt.Fprintf(c.stderr, "trajir console: http://%s/  data=%s\n", *addr, store.Root())
	if err := console.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintf(c.stderr, "trajir console: %v\n", err)
		return 1
	}
	return 0
}

// cmdMCP runs the same MCP server as trajir-mcp.
func cmdMCP(_ []string, c cliIO) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := trajirmcp.RunStdio(ctx); err != nil {
		fmt.Fprintf(c.stderr, "trajir mcp: %v\n", err)
		return 1
	}
	return 0
}

func envOr(c cliIO, key, fallback string) string {
	if v := c.getenv(key); v != "" {
		return v
	}
	return fallback
}
