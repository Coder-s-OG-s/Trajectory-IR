// Package hostinstall writes the hook and MCP config that makes an agent
// host call `trajir hook` on every tool call. It is the Go form of
// integrations/attach/project_config.py and follows its merge rules:
//
//   - existing JSON objects are merged, other keys are kept
//   - a file that is not a JSON object stops the install before any write
//   - only entries that run `trajir hook ... --host <host>` are refreshed;
//     the user's own hooks stay
//   - the Codex MCP table is appended to config.toml once
package hostinstall

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Options says where to install and what command to write.
type Options struct {
	Host    string // claude, codex, gemini, cursor, copilot
	Target  string // project dir (ignored with User)
	User    bool   // write the user-level files under Home
	Home    string // default: os.UserHomeDir
	Command string // how hooks start trajir, e.g. "trajir" or an absolute path
	DryRun  bool
}

// Hosts lists the hosts Install knows.
func Hosts() []string {
	out := make([]string, 0, len(planners))
	for k := range planners {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// file is one planned write.
type file struct {
	path string
	body []byte
}

type planner func(o Options, base string) ([]file, error)

var planners = map[string]planner{
	"claude":  planClaude,
	"codex":   planCodex,
	"gemini":  planGemini,
	"cursor":  planCursor,
	"copilot": planCopilot,
}

// Install writes the host's config and returns the paths it wrote (or would
// write, with DryRun). Every file is built first, so a bad existing file
// stops the install before anything changes.
func Install(o Options) ([]string, error) {
	plan, ok := planners[o.Host]
	if !ok {
		return nil, fmt.Errorf("unknown host %q (want one of %s)", o.Host, strings.Join(Hosts(), ", "))
	}
	if strings.TrimSpace(o.Command) == "" {
		return nil, errors.New("hostinstall: command is required")
	}
	base := o.Target
	if o.User {
		base = o.Home
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			base = home
		}
	}
	if base == "" {
		return nil, errors.New("hostinstall: target directory is required")
	}
	files, err := plan(o, base)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.path)
	}
	if o.DryRun {
		return paths, nil
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(f.path, f.body, 0o644); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// hookCommand is the shell command for one phase.
func hookCommand(o Options, phase string) string {
	return quote(o.Command) + " hook " + phase + " --host " + o.Host
}

func quote(cmd string) string {
	if strings.ContainsAny(cmd, " \t") && !strings.HasPrefix(cmd, `"`) {
		return `"` + cmd + `"`
	}
	return cmd
}

// ours reports whether a hook command is one this installer wrote.
func ours(cmd, host string) bool {
	return strings.Contains(cmd, "trajir") && strings.Contains(cmd, " hook ") && strings.HasSuffix(strings.TrimSpace(cmd), "--host "+host)
}

func readObject(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object; left it unchanged", path)
	}
	return m, nil
}

func jsonFile(path string, v any) (file, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return file{}, err
	}
	return file{path: path, body: append(raw, '\n')}, nil
}

func childObject(m map[string]any, key string) map[string]any {
	child, ok := m[key].(map[string]any)
	if !ok {
		child = map[string]any{}
		m[key] = child
	}
	return child
}

// mergeNested refreshes our hook in the Claude-style shape that Claude
// Code, Codex and Gemini share:
// {"hooks": {Event: [{"matcher": "*", "hooks": [{"type": "command", "command": ...}]}]}}
func mergeNested(cfg map[string]any, event, command, host string, extra map[string]any) {
	hooks := childObject(cfg, "hooks")
	groups, _ := hooks[event].([]any)
	entry := map[string]any{"type": "command", "command": command}
	for k, v := range extra {
		entry[k] = v
	}
	replaced := false
	for _, g := range groups {
		gm, ok := g.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := gm["hooks"].([]any)
		for i, h := range inner {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := hm["command"].(string); ours(cmd, host) {
				inner[i] = entry
				replaced = true
			}
		}
	}
	if !replaced {
		groups = append(groups, map[string]any{"matcher": "*", "hooks": []any{entry}})
	}
	hooks[event] = groups
}

// mergeFlat refreshes our hook in Cursor's shape:
// {"version": 1, "hooks": {event: [{"command": ...}]}}
func mergeFlat(cfg map[string]any, event string, entry map[string]any, host string) {
	hooks := childObject(cfg, "hooks")
	list, _ := hooks[event].([]any)
	replaced := false
	for i, h := range list {
		if hm, ok := h.(map[string]any); ok {
			if cmd, _ := hm["command"].(string); ours(cmd, host) {
				list[i] = entry
				replaced = true
			}
		}
	}
	if !replaced {
		list = append(list, entry)
	}
	hooks[event] = list
}

func mcpServer(o Options, base string) map[string]any {
	server := map[string]any{"command": o.Command, "args": []any{"mcp"}}
	if !o.User {
		server["env"] = map[string]any{"TRAJIR_MCP_ROOT": base}
	}
	return server
}

func withMCP(path string, o Options, base string) (file, error) {
	cfg, err := readObject(path)
	if err != nil {
		return file{}, err
	}
	childObject(cfg, "mcpServers")["trajectory-ir"] = mcpServer(o, base)
	return jsonFile(path, cfg)
}

func planClaude(o Options, base string) ([]file, error) {
	settings := filepath.Join(base, ".claude", "settings.local.json")
	if o.User {
		settings = filepath.Join(base, ".claude", "settings.json")
	}
	cfg, err := readObject(settings)
	if err != nil {
		return nil, err
	}
	for _, e := range [][2]string{
		{"PreToolUse", "pre"}, {"PostToolUse", "post"},
		{"PostToolUseFailure", "post-failure"}, {"SessionStart", "session-start"},
	} {
		mergeNested(cfg, e[0], hookCommand(o, e[1]), o.Host, nil)
	}
	out := []file{}
	f, err := jsonFile(settings, cfg)
	if err != nil {
		return nil, err
	}
	out = append(out, f)
	if !o.User {
		m, err := withMCP(filepath.Join(base, ".mcp.json"), o, base)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func planCodex(o Options, base string) ([]file, error) {
	path := filepath.Join(base, ".codex", "hooks.json")
	cfg, err := readObject(path)
	if err != nil {
		return nil, err
	}
	for _, e := range [][2]string{{"PreToolUse", "pre"}, {"PostToolUse", "post"}, {"SessionStart", "session-start"}} {
		mergeNested(cfg, e[0], hookCommand(o, e[1]), o.Host, map[string]any{"timeout": 30})
	}
	hooks, err := jsonFile(path, cfg)
	if err != nil {
		return nil, err
	}
	tomlPath := filepath.Join(base, ".codex", "config.toml")
	current, err := os.ReadFile(tomlPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	body := string(current)
	if !strings.Contains(body, "[mcp_servers.trajectory-ir]") {
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += "\n[mcp_servers.trajectory-ir]\n" +
			"command = " + tomlString(o.Command) + "\n" +
			"args = [\"mcp\"]\n"
		if !o.User {
			body += "\n[mcp_servers.trajectory-ir.env]\nTRAJIR_MCP_ROOT = " + tomlString(base) + "\n"
		}
	}
	return []file{hooks, {path: tomlPath, body: []byte(body)}}, nil
}

func tomlString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func planGemini(o Options, base string) ([]file, error) {
	path := filepath.Join(base, ".gemini", "settings.json")
	cfg, err := readObject(path)
	if err != nil {
		return nil, err
	}
	for _, e := range [][2]string{{"BeforeTool", "pre"}, {"AfterTool", "post"}, {"SessionStart", "session-start"}} {
		mergeNested(cfg, e[0], hookCommand(o, e[1]), o.Host, map[string]any{"name": "trajir"})
	}
	childObject(cfg, "mcpServers")["trajectory-ir"] = mcpServer(o, base)
	f, err := jsonFile(path, cfg)
	if err != nil {
		return nil, err
	}
	return []file{f}, nil
}

func planCursor(o Options, base string) ([]file, error) {
	path := filepath.Join(base, ".cursor", "hooks.json")
	cfg, err := readObject(path)
	if err != nil {
		return nil, err
	}
	if _, ok := cfg["version"]; !ok {
		cfg["version"] = 1
	}
	for _, e := range [][2]string{
		{"preToolUse", "pre"}, {"postToolUse", "post"},
		{"postToolUseFailure", "post-failure"}, {"sessionStart", "session-start"},
	} {
		entry := map[string]any{"command": hookCommand(o, e[1])}
		if e[1] == "pre" {
			// A missing or crashed trajir must block, not let the tool run unrecorded.
			entry["failClosed"] = true
		}
		mergeFlat(cfg, e[0], entry, o.Host)
	}
	hooks, err := jsonFile(path, cfg)
	if err != nil {
		return nil, err
	}
	mcp, err := withMCP(filepath.Join(base, ".cursor", "mcp.json"), o, base)
	if err != nil {
		return nil, err
	}
	return []file{hooks, mcp}, nil
}

// planCopilot owns its own file, so it is written whole.
func planCopilot(o Options, base string) ([]file, error) {
	dir := filepath.Join(base, ".github", "hooks")
	if o.User {
		dir = filepath.Join(base, ".copilot", "hooks")
	}
	hooks := map[string]any{}
	for _, e := range [][2]string{
		{"preToolUse", "pre"}, {"postToolUse", "post"},
		{"postToolUseFailure", "post-failure"}, {"sessionStart", "session-start"},
	} {
		cmd := hookCommand(o, e[1])
		hooks[e[0]] = []any{map[string]any{
			"type":       "command",
			"bash":       cmd,
			"powershell": "& " + cmd,
			"timeoutSec": 30,
		}}
	}
	f, err := jsonFile(filepath.Join(dir, "trajir.json"), map[string]any{"version": 1, "hooks": hooks})
	if err != nil {
		return nil, err
	}
	return []file{f}, nil
}
