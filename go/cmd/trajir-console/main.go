// Command trajir-console is a local append-only event ingest/read process for
// the Trajectory console (docs/CONSOLE_EVENTS.md).
//
// Environment:
//
//	TRAJIR_CONSOLE_DATA   required data directory (NDJSON under trajectories/)
//	TRAJIR_CONSOLE_TOKEN  optional Bearer token for HTTP API
//	TRAJIR_CONSOLE_ADDR   listen address (default 127.0.0.1:8787)
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/console"
)

func main() {
	fs := flag.NewFlagSet("trajir-console", flag.ExitOnError)
	addr := fs.String("addr", envOr("TRAJIR_CONSOLE_ADDR", "127.0.0.1:8787"), "listen address")
	dataDir := fs.String("data", os.Getenv("TRAJIR_CONSOLE_DATA"), "console data directory")
	token := fs.String("token", os.Getenv("TRAJIR_CONSOLE_TOKEN"), "optional Bearer token")
	_ = fs.Parse(os.Args[1:])

	if len(fs.Args()) > 0 && fs.Arg(0) == "help" {
		fmt.Fprintf(os.Stderr, "usage: trajir-console [-data DIR] [-addr HOST:PORT] [-token TOKEN]\n")
		os.Exit(0)
	}

	store, err := console.OpenStore(*dataDir)
	if err != nil {
		log.Fatalf("trajir-console: %v", err)
	}
	srv := console.NewServer(store, *token)
	log.Printf("trajir-console: listening on http://%s (data=%s)", *addr, store.Root())
	if err := console.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("trajir-console: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
