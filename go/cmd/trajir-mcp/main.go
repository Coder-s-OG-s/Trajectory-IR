// Command trajir-mcp is the Trajectory IR Model Context Protocol server (stdio).
//
// Hosts (Claude Code, Cursor, etc.) launch this binary and call tools such as
// trajectory_status and trajectory_export_tir. IR semantics live in trajir/*;
// this binary is only an adapter.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	trajirmcp "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := trajirmcp.RunStdio(ctx); err != nil {
		log.Printf("trajir-mcp: %v", err)
		os.Exit(1)
	}
}
