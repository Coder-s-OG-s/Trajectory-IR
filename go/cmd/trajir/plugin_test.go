package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The plugin's MCP server must get TRAJIR_MCP_ROOT, or every MCP tool fails
// with "TRAJIR_MCP_ROOT must be set". Claude Code fills in
// ${CLAUDE_PROJECT_DIR} in a plugin's MCP config. The server runs trajir
// directly because Claude Code starts MCP servers outside Git Bash, where
// sh may not be on PATH on Windows.
func TestPluginMCPServerSetsRoot(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "plugins", "claude-code", ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	server, ok := cfg.MCPServers["trajectory-ir"]
	if !ok {
		t.Fatal("no trajectory-ir server")
	}
	if server.Command != "trajir" || len(server.Args) != 1 || server.Args[0] != "mcp" {
		t.Fatalf("server runs %q %v, want trajir mcp", server.Command, server.Args)
	}
	if got := server.Env["TRAJIR_MCP_ROOT"]; got != "${CLAUDE_PROJECT_DIR}" {
		t.Fatalf("TRAJIR_MCP_ROOT = %q, want ${CLAUDE_PROJECT_DIR}", got)
	}
}
