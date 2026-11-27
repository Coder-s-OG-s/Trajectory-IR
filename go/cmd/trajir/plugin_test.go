package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "plugins", "grok", "hooks", "hooks.json")); err != nil {
		t.Fatal(err)
	}
	return root
}

func readTree(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

func handlers(t *testing.T, cfg map[string]any, event string) []map[string]any {
	t.Helper()
	hooks, _ := cfg["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	out := []map[string]any{}
	for _, g := range groups {
		inner, _ := g.(map[string]any)["hooks"].([]any)
		for _, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				t.Fatalf("%s handler is not an object: %#v", event, h)
			}
			out = append(out, hm)
		}
	}
	return out
}

func TestGrokPluginContract(t *testing.T) {
	root := repoRoot(t)
	manifest := readTree(t, filepath.Join(root, "plugins", "grok", ".grok-plugin", "plugin.json"))
	if manifest["name"] != "trajectory-ir-grok" || manifest["version"] != "0.1.0" {
		t.Fatalf("manifest = %v", manifest)
	}
	market := readTree(t, filepath.Join(root, ".claude-plugin", "marketplace.json"))
	plugins, _ := market["plugins"].([]any)
	var claude, grok map[string]any
	for _, p := range plugins {
		pm := p.(map[string]any)
		switch pm["name"] {
		case "trajectory-ir":
			claude = pm
		case "trajectory-ir-grok":
			grok = pm
		}
	}
	if claude["source"] != "./plugins/claude-code" {
		t.Fatalf("claude marketplace entry = %v", claude)
	}
	if grok["source"] != "./plugins/grok" {
		t.Fatalf("grok marketplace entry = %v", grok)
	}

	cfg := readTree(t, filepath.Join(root, "plugins", "grok", "hooks", "hooks.json"))
	wantArgs := map[string]string{
		"PreToolUse":         "pre",
		"PostToolUse":        "post",
		"PostToolUseFailure": "post-failure",
		"SessionStart":       "session-start",
	}
	for event, phase := range wantArgs {
		got := handlers(t, cfg, event)
		if len(got) != 1 {
			t.Fatalf("%s handlers = %v", event, got)
		}
		h := got[0]
		for _, banned := range []string{"if", "async", "shell", "statusMessage", "headers"} {
			if _, ok := h[banned]; ok {
				t.Fatalf("%s sets %s", event, banned)
			}
		}
		if h["type"] != "command" || h["command"] != "trajir" {
			t.Fatalf("%s command = %v", event, h)
		}
		if strings.Contains(h["command"].(string), "sh") {
			t.Fatalf("%s command uses a shell: %v", event, h["command"])
		}
		args, _ := h["args"].([]any)
		joined := make([]string, 0, len(args))
		for _, a := range args {
			joined = append(joined, a.(string))
		}
		want := "hook " + phase + " --host grok --owner plugin"
		if strings.Join(joined, " ") != want {
			t.Fatalf("%s args = %v", event, joined)
		}
		_, closed := h["failClosed"]
		if event == "PreToolUse" {
			if h["failClosed"] != true || h["timeout"] != float64(30) {
				t.Fatalf("pre = %v", h)
			}
		} else if closed {
			t.Fatalf("%s must not set failClosed: %v", event, h)
		}
	}
	if handlers(t, cfg, "PostToolUse")[0]["timeout"] != float64(60) || handlers(t, cfg, "PostToolUseFailure")[0]["timeout"] != float64(60) {
		t.Fatal("post timeouts")
	}
	if handlers(t, cfg, "SessionStart")[0]["timeout"] != float64(30) {
		t.Fatal("session-start timeout")
	}

	mcp := readTree(t, filepath.Join(root, "plugins", "grok", ".mcp.json"))
	server := mcp["mcpServers"].(map[string]any)["trajectory-ir"].(map[string]any)
	env := server["env"].(map[string]any)
	if server["command"] != "trajir" || env["TRAJIR_MCP_ROOT"] != "${GROK_WORKSPACE_ROOT}" {
		t.Fatalf("mcp = %v", server)
	}

	runs, err := os.ReadFile(filepath.Join(root, "plugins", "grok", "skills", "trajir-runs", "SKILL.md"))
	if err != nil || !strings.Contains(string(runs), "grok-") || strings.Contains(string(runs), "TRAJIR_BIN") || strings.Contains(string(runs), "exactly-once") || strings.Contains(string(runs), "exactly once") {
		t.Fatalf("runs skill: %v", err)
	}
	dash, err := os.ReadFile(filepath.Join(root, "plugins", "grok", "skills", "trajir-dashboard", "SKILL.md"))
	if err != nil || !strings.Contains(string(dash), "grok-cli") {
		t.Fatalf("dashboard skill: %v", err)
	}
}
