package main

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/console"
)

// hostCall builds one host's pre, post and failure payloads for a call, in
// the shapes from that host's hook reference (see trajir/hosthook/hosts).
type hostCall func(h *harness, id, tool string, input map[string]any) (pre, post, failure string, failurePhase string)

var hostCalls = map[string]hostCall{
	"claude": func(h *harness, id, tool string, in map[string]any) (string, string, string, string) {
		base := map[string]any{"session_id": "e2e", "tool_use_id": id, "tool_name": tool, "tool_input": in}
		return h.payload(base), h.payload(with(base, "tool_response", "ok")), h.payload(with(base, "error", "exit 1")), "post-failure"
	},
	"codex": func(h *harness, id, tool string, in map[string]any) (string, string, string, string) {
		base := map[string]any{"session_id": "e2e", "tool_name": tool, "tool_input": in}
		pre := with(base, "tool_use_id", id)
		// Codex has no failure hook: a failed command is a PostToolUse result.
		return h.payload(pre), h.payload(with(base, "tool_response", "ok")), h.payload(with(base, "tool_response", "exit 1")), "post"
	},
	"gemini": func(h *harness, _, tool string, in map[string]any) (string, string, string, string) {
		base := map[string]any{"session_id": "e2e", "tool_name": tool, "tool_input": in}
		return h.payload(base),
			h.payload(with(base, "tool_response", map[string]any{"llmContent": "ok"})),
			h.payload(with(base, "tool_response", map[string]any{"llmContent": "", "error": "boom"})), "post"
	},
	"cursor": func(h *harness, id, tool string, in map[string]any) (string, string, string, string) {
		base := map[string]any{"conversation_id": "e2e", "tool_use_id": id, "tool_name": tool, "tool_input": in}
		return h.payload(base), h.payload(with(base, "tool_output", `{"ok":true}`)),
			h.payload(with(with(base, "error_message", "boom"), "failure_type", "error")), "post-failure"
	},
	"copilot": func(h *harness, _, tool string, in map[string]any) (string, string, string, string) {
		raw, _ := json.Marshal(in)
		base := map[string]any{"sessionId": "e2e", "toolName": tool, "toolArgs": string(raw)}
		return h.payload(base),
			h.payload(with(base, "toolResult", map[string]any{"resultType": "success", "textResultForLlm": "ok"})),
			h.payload(with(base, "error", "boom")), "post-failure"
	},
}

func with(m map[string]any, k string, v any) map[string]any {
	out := map[string]any{k: v}
	for key, val := range m {
		if key != k {
			out[key] = val
		}
	}
	return out
}

// Every host: two successful calls and one failed call, then export,
// verify, and the dashboard shows the run under that host's runtime.
func TestEveryHostRecordsExportsAndShowsOnDashboard(t *testing.T) {
	runtimes := map[string]string{"claude": "claude-code", "codex": "codex", "gemini": "gemini-cli", "cursor": "cursor", "copilot": "copilot-cli"}
	for host, call := range hostCalls {
		t.Run(host, func(t *testing.T) {
			h := newHarness(t)
			for i, tool := range []string{"read_file", "deploy", "deploy_again"} {
				id := "c" + string(rune('1'+i))
				pre, post, failure, failPhase := call(h, id, tool, map[string]any{"n": i})
				h.mustRun(pre, "hook", "pre", "--host", host)
				if i == 2 {
					h.mustRun(failure, "hook", failPhase, "--host", host)
				} else {
					h.mustRun(post, "hook", "post", "--host", host)
				}
			}
			traj := host + "-e2e"
			c := h.counts(host, traj)
			results, aborts := 2, 1
			if host == "codex" {
				// No failure signal from Codex: the failed command is a result.
				results, aborts = 3, 0
			}
			if c["DECISION"] != 3 || c["TOOL_RESULT"] != results || c["ABORT"] != aborts || c["COMMIT_STEP"] != 3 {
				t.Fatalf("counts = %v", c)
			}
			pack := filepath.Join(h.project, "run.tir")
			h.mustRun("", "export", "--project", h.project, "--host", host, "--trajectory", traj, "-o", pack)
			if out := h.mustRun("", "verify", pack); !strings.HasPrefix(out, "OK") {
				t.Fatalf("verify %q", out)
			}
			store, err := console.OpenStore(h.data)
			if err != nil {
				t.Fatal(err)
			}
			dash, err := store.Dashboard()
			if err != nil {
				t.Fatal(err)
			}
			if len(dash.Runs) != 1 || dash.Runs[0].Runtime != runtimes[host] || dash.Runs[0].SealCreatedCount != 3 || dash.Runs[0].ExportsOK != 1 {
				t.Fatalf("dashboard runs = %+v", dash.Runs)
			}
		})
	}
}

// BenchmarkHookPrePost is one sealed and recorded tool call through the
// CLI path a host runs, on a warm work dir. Target: p50 under 50 ms.
func BenchmarkHookPrePost(b *testing.B) {
	h := newHarness(b)
	h.env["TRAJIR_CONSOLE_SINK"] = "off"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := "b" + strconv.Itoa(i)
		in := map[string]any{"i": i}
		pre := claudeEvent(h, id, "Bash", in, nil)
		post := claudeEvent(h, id, "Bash", in, map[string]any{"tool_response": "ok"})
		if code, _, errOut := h.run(pre, "hook", "pre", "--host", "claude"); code != 0 {
			b.Fatalf("pre exit %d: %s", code, errOut)
		}
		if code, _, errOut := h.run(post, "hook", "post", "--host", "claude"); code != 0 || errOut != "" {
			b.Fatalf("post exit %d: %s", code, errOut)
		}
	}
}
