// Package hosts turns each agent host's hook payload into a
// hosthook.Event, and writes that host's way of blocking a tool call.
//
// Payload shapes follow each host's hook reference as of 2026-11:
//
//	claude   https://code.claude.com/docs/en/hooks
//	codex    https://developers.openai.com/codex/hooks
//	gemini   https://geminicli.com/docs/hooks/reference/
//	cursor   https://cursor.com/docs/agent/hooks
//	copilot  https://docs.github.com/en/copilot/reference/hooks-reference
package hosts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
)

// Adapter is one host. Name is the short key used in paths and trajectory
// ids. Runtime labels console events.
type Adapter interface {
	Name() string
	Runtime() string
	Parse(phase hosthook.Phase, raw []byte) (hosthook.Event, error)
	// Block refuses the tool call and returns the exit code.
	Block(stdout, stderr io.Writer, reason string) int
}

var registry = map[string]Adapter{}

func register(a Adapter) { registry[a.Name()] = a }

func init() {
	register(claude{})
	register(codex{})
	register(gemini{})
	register(cursor{})
	register(copilot{})
}

// Lookup returns the adapter for a host name.
func Lookup(name string) (Adapter, error) {
	a, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("unknown host %q (want one of %s)", name, strings.Join(Names(), ", "))
	}
	return a, nil
}

// Names lists the known hosts, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// payload is a decoded hook body. Numbers stay json.Number.
type payload map[string]any

func decode(raw []byte) (payload, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var p payload
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("hook payload is not a JSON object: %w", err)
	}
	if p == nil {
		return nil, fmt.Errorf("hook payload is empty")
	}
	return p, nil
}

func (p payload) str(keys ...string) string {
	for _, k := range keys {
		switch v := p[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case json.Number:
			return v.String()
		}
	}
	return ""
}

func (p payload) obj(key string) map[string]any {
	m, _ := p[key].(map[string]any)
	return m
}

// args makes tool input an object. A JSON string holding an object is
// decoded (Copilot sends toolArgs that way). Anything else is wrapped as
// {"value": x}, like the Python Claude hook.
func args(v any) map[string]any {
	switch t := v.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return t
	case string:
		if p, err := decode([]byte(t)); err == nil {
			return p
		}
	}
	return map[string]any{"value": v}
}

func annotations(p payload) map[string]any {
	if m := p.obj("annotations"); m != nil {
		return m
	}
	return p.obj("tool_annotations")
}

// stderrBlock is the common block: exit 2 with the reason on stderr.
func stderrBlock(stderr io.Writer, reason string) int {
	fmt.Fprintf(stderr, "trajir: %s\n", reason)
	return 2
}

// jsonBlock writes a deny object on stdout, the reason on stderr, and exits 2.
func jsonBlock(stdout, stderr io.Writer, deny map[string]any, reason string) int {
	raw, _ := json.Marshal(deny)
	fmt.Fprintln(stdout, string(raw))
	return stderrBlock(stderr, reason)
}

// claudeShape parses the Claude Code payload, which Codex also uses.
func claudeShape(host string, phase hosthook.Phase, raw []byte) (hosthook.Event, error) {
	p, err := decode(raw)
	if err != nil {
		return hosthook.Event{}, err
	}
	ev := hosthook.Event{
		Host:        host,
		SessionID:   p.str("session_id"),
		ToolUseID:   p.str("tool_use_id"),
		ToolName:    p.str("tool_name"),
		Cwd:         p.str("cwd"),
		Args:        args(p["tool_input"]),
		Annotations: annotations(p),
		Result:      p["tool_response"],
	}
	if phase == hosthook.PhasePostFailure {
		ev.Error = p["error"]
	}
	return ev, nil
}

type claude struct{}

func (claude) Name() string    { return "claude" }
func (claude) Runtime() string { return "claude-code" }
func (claude) Parse(phase hosthook.Phase, raw []byte) (hosthook.Event, error) {
	return claudeShape("claude", phase, raw)
}
func (claude) Block(_, stderr io.Writer, reason string) int { return stderrBlock(stderr, reason) }

// codex uses the Claude payload. PostToolUse may leave out tool_use_id, and
// there is no failure hook: a failed shell command is still a result.
type codex struct{}

func (codex) Name() string    { return "codex" }
func (codex) Runtime() string { return "codex" }
func (codex) Parse(phase hosthook.Phase, raw []byte) (hosthook.Event, error) {
	return claudeShape("codex", phase, raw)
}
func (codex) Block(_, stderr io.Writer, reason string) int { return stderrBlock(stderr, reason) }

// gemini sends no tool call id. A failed tool is AfterTool with
// tool_response.error set.
type gemini struct{}

func (gemini) Name() string    { return "gemini" }
func (gemini) Runtime() string { return "gemini-cli" }
func (gemini) Parse(phase hosthook.Phase, raw []byte) (hosthook.Event, error) {
	ev, err := claudeShape("gemini", phase, raw)
	if err != nil {
		return ev, err
	}
	ev.ToolUseID = ""
	if resp, ok := ev.Result.(map[string]any); ok {
		if msg, _ := resp["error"].(string); msg != "" {
			ev.Failed, ev.Error = true, msg
		}
	}
	return ev, nil
}

// Block uses exit 2. Gemini sends stderr to the agent as the reason and the
// turn goes on.
func (gemini) Block(_, stderr io.Writer, reason string) int { return stderrBlock(stderr, reason) }

// cursor keys the session as conversation_id and sends tool_output as a
// JSON string.
type cursor struct{}

func (cursor) Name() string    { return "cursor" }
func (cursor) Runtime() string { return "cursor" }
func (cursor) Parse(phase hosthook.Phase, raw []byte) (hosthook.Event, error) {
	p, err := decode(raw)
	if err != nil {
		return hosthook.Event{}, err
	}
	ev := hosthook.Event{
		Host:      "cursor",
		SessionID: p.str("conversation_id", "session_id"),
		ToolUseID: p.str("tool_use_id"),
		ToolName:  p.str("tool_name"),
		Cwd:       p.str("cwd"),
		Args:      args(p["tool_input"]),
	}
	if ev.Cwd == "" {
		if roots, ok := p["workspace_roots"].([]any); ok && len(roots) > 0 {
			ev.Cwd, _ = roots[0].(string)
		}
	}
	if out, ok := p["tool_output"].(string); ok {
		if parsed, err := decode([]byte(out)); err == nil {
			ev.Result = map[string]any(parsed)
		} else {
			ev.Result = out
		}
	} else {
		ev.Result = p["tool_output"]
	}
	if phase == hosthook.PhasePostFailure {
		msg := p.str("error_message")
		if kind := p.str("failure_type"); kind != "" {
			msg = kind + ": " + msg
		}
		ev.Error = msg
	}
	return ev, nil
}
func (cursor) Block(stdout, stderr io.Writer, reason string) int {
	return jsonBlock(stdout, stderr, map[string]any{
		"permission":    "deny",
		"user_message":  reason,
		"agent_message": reason,
	}, reason)
}

// copilot sends camelCase fields with toolArgs as a JSON string, or the
// VS Code snake_case shape when the config uses PascalCase event names.
// There is no tool call id.
type copilot struct{}

func (copilot) Name() string    { return "copilot" }
func (copilot) Runtime() string { return "copilot-cli" }
func (copilot) Parse(phase hosthook.Phase, raw []byte) (hosthook.Event, error) {
	p, err := decode(raw)
	if err != nil {
		return hosthook.Event{}, err
	}
	input, ok := p["toolArgs"]
	if !ok {
		input = p["tool_input"]
	}
	ev := hosthook.Event{
		Host:      "copilot",
		SessionID: p.str("sessionId", "session_id"),
		ToolName:  p.str("toolName", "tool_name"),
		Cwd:       p.str("cwd"),
		Args:      args(input),
	}
	result := p.obj("toolResult")
	if result == nil {
		result = p.obj("tool_result")
	}
	if result != nil {
		ev.Result = result
		kind, _ := result["resultType"].(string)
		if kind == "" {
			kind, _ = result["result_type"].(string)
		}
		if kind != "" && kind != "success" {
			text, _ := result["textResultForLlm"].(string)
			if text == "" {
				text, _ = result["text_result_for_llm"].(string)
			}
			ev.Failed, ev.Error = true, text
		}
	}
	if phase == hosthook.PhasePostFailure {
		ev.Error = p["error"]
	}
	return ev, nil
}
func (copilot) Block(stdout, stderr io.Writer, reason string) int {
	return jsonBlock(stdout, stderr, map[string]any{
		"permissionDecision":       "deny",
		"permissionDecisionReason": reason,
	}, reason)
}
