package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginMCPServerSetsRoot(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for {
		candidate := filepath.Join(dir, "plugins", "claude-code", ".mcp.json")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("plugins/claude-code/.mcp.json not found")
		}
		dir = parent
	}
	raw, err := os.ReadFile(path)
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
	server := cfg.MCPServers["trajectory-ir"]
	if server.Command != "trajir" || len(server.Args) != 1 || server.Args[0] != "mcp" {
		t.Fatalf("server = %+v", server)
	}
	if server.Env["TRAJIR_MCP_ROOT"] != "${CLAUDE_PROJECT_DIR}" {
		t.Fatalf("TRAJIR_MCP_ROOT = %q", server.Env["TRAJIR_MCP_ROOT"])
	}
}
