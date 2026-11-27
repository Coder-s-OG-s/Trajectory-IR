package hostinstall_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hostinstall"
)

func readJSON(t *testing.T, path string) map[string]any {
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

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func install(t *testing.T, o hostinstall.Options) []string {
	t.Helper()
	if o.Command == "" {
		o.Command = "trajir"
	}
	paths, err := hostinstall.Install(o)
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

// commands collects every hook command in a Claude-style hooks object.
func commands(t *testing.T, cfg map[string]any, event string) []string {
	t.Helper()
	hooks, _ := cfg["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	out := []string{}
	for _, g := range groups {
		gm := g.(map[string]any)
		if inner, ok := gm["hooks"].([]any); ok {
			for _, h := range inner {
				out = append(out, h.(map[string]any)["command"].(string))
			}
		} else if cmd, ok := gm["command"].(string); ok {
			out = append(out, cmd)
		} else if cmd, ok := gm["bash"].(string); ok {
			out = append(out, cmd)
		}
	}
	return out
}

func TestInstallCodexWritesHooksAndMCP(t *testing.T) {
	dir := t.TempDir()
	install(t, hostinstall.Options{Host: "codex", Target: dir})
	hooks := readJSON(t, filepath.Join(dir, ".codex", "hooks.json"))
	if got := commands(t, hooks, "PreToolUse"); len(got) != 1 || got[0] != "trajir hook pre --host codex --owner project" {
		t.Fatalf("PreToolUse = %v", got)
	}
	if got := commands(t, hooks, "PostToolUse"); len(got) != 1 || got[0] != "trajir hook post --host codex --owner project" {
		t.Fatalf("PostToolUse = %v", got)
	}
	toml, err := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	if err != nil || !strings.Contains(string(toml), "[mcp_servers.trajectory-ir]") || !strings.Contains(string(toml), `args = ["mcp"]`) {
		t.Fatalf("config.toml = %q, %v", toml, err)
	}
}

func TestCodexTomlAppendOnce(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".codex", "config.toml"), "model = \"o3\"\n")
	install(t, hostinstall.Options{Host: "codex", Target: dir})
	install(t, hostinstall.Options{Host: "codex", Target: dir})
	raw, _ := os.ReadFile(filepath.Join(dir, ".codex", "config.toml"))
	if strings.Count(string(raw), "[mcp_servers.trajectory-ir]") != 1 || !strings.HasPrefix(string(raw), "model = \"o3\"") {
		t.Fatalf("config.toml = %q", raw)
	}
}

func TestMergeKeepsUserHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.local.json")
	write(t, path, `{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"lint.sh"}]}]}}`)
	install(t, hostinstall.Options{Host: "claude", Target: dir})
	cfg := readJSON(t, path)
	if cfg["model"] != "opus" {
		t.Fatalf("other settings lost: %v", cfg)
	}
	got := commands(t, cfg, "PreToolUse")
	if len(got) != 2 || got[0] != "lint.sh" || got[1] != "trajir hook pre --host claude --owner project" {
		t.Fatalf("PreToolUse = %v", got)
	}
	if got := commands(t, cfg, "PostToolUseFailure"); len(got) != 1 {
		t.Fatalf("PostToolUseFailure = %v", got)
	}
	mcp := readJSON(t, filepath.Join(dir, ".mcp.json"))
	server := mcp["mcpServers"].(map[string]any)["trajectory-ir"].(map[string]any)
	if server["command"] != "trajir" || server["args"].([]any)[0] != "mcp" {
		t.Fatalf("mcp server = %v", server)
	}
}

func TestRefreshesOwnEntry(t *testing.T) {
	dir := t.TempDir()
	install(t, hostinstall.Options{Host: "claude", Target: dir, Command: "/old/trajir"})
	install(t, hostinstall.Options{Host: "claude", Target: dir, Command: "/new path/trajir"})
	got := commands(t, readJSON(t, filepath.Join(dir, ".claude", "settings.local.json")), "PreToolUse")
	if len(got) != 1 || got[0] != `"/new path/trajir" hook pre --host claude --owner project` {
		t.Fatalf("PreToolUse = %v", got)
	}
}

func TestNonObjectFileNoWrite(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".cursor", "mcp.json"), "[]")
	if _, err := hostinstall.Install(hostinstall.Options{Host: "cursor", Target: dir, Command: "trajir"}); err == nil || !strings.Contains(err.Error(), "mcp.json") {
		t.Fatalf("want error naming mcp.json, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cursor", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("hooks.json written despite error: %v", err)
	}
}

func TestCursorShape(t *testing.T) {
	dir := t.TempDir()
	install(t, hostinstall.Options{Host: "cursor", Target: dir})
	cfg := readJSON(t, filepath.Join(dir, ".cursor", "hooks.json"))
	if cfg["version"] != float64(1) {
		t.Fatalf("version = %v", cfg["version"])
	}
	pre := cfg["hooks"].(map[string]any)["preToolUse"].([]any)[0].(map[string]any)
	if pre["command"] != "trajir hook pre --host cursor --owner project" || pre["failClosed"] != true {
		t.Fatalf("preToolUse = %v", pre)
	}
	if got := commands(t, cfg, "postToolUseFailure"); len(got) != 1 {
		t.Fatalf("postToolUseFailure = %v", got)
	}
}

func TestCopilotShape(t *testing.T) {
	dir := t.TempDir()
	install(t, hostinstall.Options{Host: "copilot", Target: dir})
	cfg := readJSON(t, filepath.Join(dir, ".github", "hooks", "trajir.json"))
	pre := cfg["hooks"].(map[string]any)["preToolUse"].([]any)[0].(map[string]any)
	if pre["bash"] != "trajir hook pre --host copilot --owner project" || pre["powershell"] != "& trajir hook pre --host copilot --owner project" || pre["type"] != "command" {
		t.Fatalf("preToolUse = %v", pre)
	}
}

func TestGeminiShape(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".gemini", "settings.json"), `{"mcpServers":{"other":{"command":"x"}}}`)
	install(t, hostinstall.Options{Host: "gemini", Target: dir})
	cfg := readJSON(t, filepath.Join(dir, ".gemini", "settings.json"))
	if got := commands(t, cfg, "BeforeTool"); len(got) != 1 || got[0] != "trajir hook pre --host gemini --owner project" {
		t.Fatalf("BeforeTool = %v", got)
	}
	servers := cfg["mcpServers"].(map[string]any)
	if servers["other"] == nil || servers["trajectory-ir"] == nil {
		t.Fatalf("mcpServers = %v", servers)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	paths := install(t, hostinstall.Options{Host: "codex", Target: dir, DryRun: true})
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("dry run wrote %v", entries)
	}
}

func TestUserScopeUsesHome(t *testing.T) {
	home := t.TempDir()
	paths := install(t, hostinstall.Options{Host: "cursor", User: true, Home: home})
	if _, err := os.Stat(filepath.Join(home, ".cursor", "hooks.json")); err != nil {
		t.Fatalf("user hooks not written: %v (paths %v)", err, paths)
	}
}

func TestUnknownHost(t *testing.T) {
	if _, err := hostinstall.Install(hostinstall.Options{Host: "nope", Target: t.TempDir(), Command: "trajir"}); err == nil {
		t.Fatal("unknown host accepted")
	}
}

func TestOwnerNamesScope(t *testing.T) {
	dir := t.TempDir()
	install(t, hostinstall.Options{Host: "codex", Target: dir})
	if got := commands(t, readJSON(t, filepath.Join(dir, ".codex", "hooks.json")), "PreToolUse"); got[0] != "trajir hook pre --host codex --owner project" {
		t.Fatalf("project command = %v", got)
	}
	home := t.TempDir()
	install(t, hostinstall.Options{Host: "codex", User: true, Home: home})
	if got := commands(t, readJSON(t, filepath.Join(home, ".codex", "hooks.json")), "PreToolUse"); got[0] != "trajir hook pre --host codex --owner user" {
		t.Fatalf("user command = %v", got)
	}
}

func TestGrokShape(t *testing.T) {
	dir := t.TempDir()
	install(t, hostinstall.Options{Host: "grok", Target: dir, Command: `C:\Program Files\trajir.exe`})
	cfg := readJSON(t, filepath.Join(dir, ".grok", "hooks", "trajir.json"))
	pre := cfg["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if pre["command"] != `C:\Program Files\trajir.exe` || pre["failClosed"] != true || pre["timeout"] != float64(30) {
		t.Fatalf("pre = %v", pre)
	}
	args := pre["args"].([]any)
	if len(args) != 6 || args[0] != "hook" || args[1] != "pre" || args[2] != "--host" || args[3] != "grok" || args[4] != "--owner" || args[5] != "project" {
		t.Fatalf("args = %v", args)
	}
	post := cfg["hooks"].(map[string]any)["PostToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if _, ok := post["failClosed"]; ok || post["timeout"] != float64(60) {
		t.Fatalf("post = %v", post)
	}
	fail := cfg["hooks"].(map[string]any)["PostToolUseFailure"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if _, ok := fail["failClosed"]; ok || fail["timeout"] != float64(60) {
		t.Fatalf("post-failure = %v", fail)
	}
	start := cfg["hooks"].(map[string]any)["SessionStart"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if _, ok := start["failClosed"]; ok || start["timeout"] != float64(30) {
		t.Fatalf("session-start = %v", start)
	}
	toml, err := os.ReadFile(filepath.Join(dir, ".grok", "config.toml"))
	if err != nil || !strings.Contains(string(toml), "[mcp_servers.trajectory-ir]") || !strings.Contains(string(toml), "TRAJIR_MCP_ROOT = ") || !strings.Contains(string(toml), filepath.Base(dir)) {
		t.Fatalf("config.toml = %q, %v", toml, err)
	}
}

func TestGrokUserInstallDoesNotPinHome(t *testing.T) {
	home := t.TempDir()
	install(t, hostinstall.Options{Host: "grok", User: true, Home: home})
	toml, err := os.ReadFile(filepath.Join(home, ".grok", "config.toml"))
	if err != nil || strings.Contains(string(toml), "TRAJIR_MCP_ROOT") {
		t.Fatalf("config.toml = %q, %v", toml, err)
	}
	cfg := readJSON(t, filepath.Join(home, ".grok", "hooks", "trajir.json"))
	pre := cfg["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if pre["args"].([]any)[5] != "user" {
		t.Fatalf("owner args = %v", pre["args"])
	}
}

func TestGrokMergeKeepsOtherHooksAndRefreshes(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".grok", "hooks", "trajir.json"), `{"hooks":{"PreToolUse":[{"matcher":"read_file","hooks":[{"type":"command","command":"lint.exe","args":[]}]}]}}`)
	write(t, filepath.Join(dir, ".grok", "config.toml"), "model = \"grok\"\n")
	install(t, hostinstall.Options{Host: "grok", Target: dir, Command: "trajir"})
	install(t, hostinstall.Options{Host: "grok", Target: dir, Command: `D:\tools\trajir.exe`})
	cfg := readJSON(t, filepath.Join(dir, ".grok", "hooks", "trajir.json"))
	groups := cfg["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(groups) != 2 {
		t.Fatalf("groups = %v", groups)
	}
	kept := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if kept["command"] != "lint.exe" {
		t.Fatalf("user hook = %v", kept)
	}
	ours := groups[1].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if ours["command"] != `D:\tools\trajir.exe` {
		t.Fatalf("refreshed = %v", ours)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".grok", "config.toml"))
	if strings.Count(string(raw), "[mcp_servers.trajectory-ir]") != 1 || !strings.HasPrefix(string(raw), "model = \"grok\"") {
		t.Fatalf("config.toml = %q", raw)
	}
}

func TestGrokBadHooksWritesNothing(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".grok", "hooks", "trajir.json"), "[]")
	if _, err := hostinstall.Install(hostinstall.Options{Host: "grok", Target: dir, Command: "trajir"}); err == nil || !strings.Contains(err.Error(), "trajir.json") {
		t.Fatalf("want error naming trajir.json, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".grok", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("config.toml written despite error: %v", err)
	}
}

func TestRefreshesEntryWrittenWithoutOwner(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, ".codex", "hooks.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"trajir hook pre --host codex"}]}]}}`)
	install(t, hostinstall.Options{Host: "codex", Target: dir})
	if got := commands(t, readJSON(t, filepath.Join(dir, ".codex", "hooks.json")), "PreToolUse"); len(got) != 1 {
		t.Fatalf("PreToolUse = %v", got)
	}
}
