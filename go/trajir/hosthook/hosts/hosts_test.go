package hosts_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook/hosts"
)

func parse(t *testing.T, host string, phase hosthook.Phase, fixture string) hosthook.Event {
	t.Helper()
	a, err := hosts.Lookup(host)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", host, fixture))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := a.Parse(phase, raw)
	if err != nil {
		t.Fatalf("%s/%s: %v", host, fixture, err)
	}
	return ev
}

func num(v any) string {
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	return ""
}

func TestLookupKnownHosts(t *testing.T) {
	for _, name := range []string{"claude", "codex", "gemini", "cursor", "copilot"} {
		a, err := hosts.Lookup(name)
		if err != nil || a.Name() != name || a.Runtime() == "" {
			t.Fatalf("%s: %v %v", name, a, err)
		}
	}
	if _, err := hosts.Lookup("nope"); err == nil {
		t.Fatal("unknown host accepted")
	}
}

func TestClaudeParse(t *testing.T) {
	pre := parse(t, "claude", hosthook.PhasePre, "pre.json")
	if pre.SessionID != "abc123" || pre.ToolUseID != "toolu_01" || pre.ToolName != "Bash" || pre.Cwd != "/proj" || pre.Args["command"] != "npm test" {
		t.Fatalf("pre = %+v", pre)
	}
	post := parse(t, "claude", hosthook.PhasePost, "post.json")
	if post.Result.(map[string]any)["stdout"] != "ok" || post.Failed {
		t.Fatalf("post = %+v", post)
	}
	fail := parse(t, "claude", hosthook.PhasePostFailure, "post-failure.json")
	if fail.Error != "exit status 1" || fail.ToolUseID != "toolu_01" {
		t.Fatalf("failure = %+v", fail)
	}
}

func TestCodexParse(t *testing.T) {
	pre := parse(t, "codex", hosthook.PhasePre, "pre.json")
	if pre.SessionID != "019a-codex" || pre.ToolUseID != "call_9" || pre.Args["command"] != "ls -la" {
		t.Fatalf("pre = %+v", pre)
	}
	post := parse(t, "codex", hosthook.PhasePost, "post.json")
	if post.ToolUseID != "" || post.Result != "total 0" {
		t.Fatalf("post = %+v", post)
	}
}

func TestGeminiParse(t *testing.T) {
	pre := parse(t, "gemini", hosthook.PhasePre, "pre.json")
	if pre.SessionID != "g-55" || pre.ToolUseID != "" || pre.ToolName != "read_file" || pre.Args["absolute_path"] != "/proj/a.txt" {
		t.Fatalf("pre = %+v", pre)
	}
	post := parse(t, "gemini", hosthook.PhasePost, "post.json")
	if post.Failed || post.Result.(map[string]any)["llmContent"] != "hello" {
		t.Fatalf("post = %+v", post)
	}
	bad := parse(t, "gemini", hosthook.PhasePost, "post-error.json")
	if !bad.Failed || bad.Error != "File not found" {
		t.Fatalf("error post = %+v", bad)
	}
}

func TestCursorParse(t *testing.T) {
	pre := parse(t, "cursor", hosthook.PhasePre, "pre.json")
	if pre.SessionID != "conv-7" || pre.ToolUseID != "tu-3" || pre.ToolName != "Shell" || pre.Cwd != "/proj" {
		t.Fatalf("pre = %+v", pre)
	}
	post := parse(t, "cursor", hosthook.PhasePost, "post.json")
	if r, ok := post.Result.(map[string]any); !ok || r["output"] != "clean" || num(r["exitCode"]) != "0" {
		t.Fatalf("post result = %#v", post.Result)
	}
	fail := parse(t, "cursor", hosthook.PhasePostFailure, "post-failure.json")
	if fail.Error != "timeout: command timed out" {
		t.Fatalf("failure = %+v", fail)
	}
}

func TestCursorCwdFallsBackToWorkspaceRoot(t *testing.T) {
	a, _ := hosts.Lookup("cursor")
	ev, err := a.Parse(hosthook.PhasePre, []byte(`{"conversation_id":"c","workspace_roots":["/root1"],"tool_name":"Read","tool_input":{}}`))
	if err != nil || ev.Cwd != "/root1" {
		t.Fatalf("ev = %+v, %v", ev, err)
	}
}

func TestCopilotParse(t *testing.T) {
	pre := parse(t, "copilot", hosthook.PhasePre, "pre.json")
	if pre.SessionID != "cp-1" || pre.ToolUseID != "" || pre.ToolName != "bash" || pre.Args["command"] != "make" {
		t.Fatalf("pre = %+v", pre)
	}
	post := parse(t, "copilot", hosthook.PhasePost, "post.json")
	if post.Failed || post.Result.(map[string]any)["textResultForLlm"] != "built" || post.Args["command"] != "make" {
		t.Fatalf("post = %+v", post)
	}
	fail := parse(t, "copilot", hosthook.PhasePostFailure, "post-failure.json")
	if fail.Error != "make: *** [all] Error 2" {
		t.Fatalf("failure = %+v", fail)
	}
	vs := parse(t, "copilot", hosthook.PhasePre, "pre-vscode.json")
	if vs.SessionID != "cp-2" || vs.Args["command"] != "make" {
		t.Fatalf("vscode pre = %+v", vs)
	}
}

func TestCopilotFailedResultType(t *testing.T) {
	a, _ := hosts.Lookup("copilot")
	ev, err := a.Parse(hosthook.PhasePost, []byte(`{"sessionId":"s","toolName":"bash","toolArgs":{},"toolResult":{"resultType":"failure","textResultForLlm":"boom"}}`))
	if err != nil || !ev.Failed || ev.Error != "boom" {
		t.Fatalf("ev = %+v, %v", ev, err)
	}
}

func TestNonObjectInputIsWrapped(t *testing.T) {
	a, _ := hosts.Lookup("claude")
	ev, err := a.Parse(hosthook.PhasePre, []byte(`{"session_id":"s","tool_name":"x","tool_input":"raw","tool_use_id":"1"}`))
	if err != nil || ev.Args["value"] != "raw" {
		t.Fatalf("ev = %+v, %v", ev, err)
	}
}

func TestBadJSONIsError(t *testing.T) {
	for _, name := range []string{"claude", "codex", "gemini", "cursor", "copilot"} {
		a, _ := hosts.Lookup(name)
		if _, err := a.Parse(hosthook.PhasePre, []byte("{nope")); err == nil {
			t.Fatalf("%s accepted bad json", name)
		}
	}
}

func TestBlockResponses(t *testing.T) {
	cases := map[string]string{
		"claude":  "",
		"codex":   "",
		"gemini":  "",
		"cursor":  `{"agent_message":"no","permission":"deny","user_message":"no"}`,
		"copilot": `{"permissionDecision":"deny","permissionDecisionReason":"no"}`,
	}
	for name, wantOut := range cases {
		a, _ := hosts.Lookup(name)
		var out, errOut bytes.Buffer
		code := a.Block(&out, &errOut, "no")
		if code != 2 {
			t.Fatalf("%s exit = %d", name, code)
		}
		if strings.TrimSpace(out.String()) != wantOut {
			t.Fatalf("%s stdout = %q", name, out.String())
		}
		if !strings.Contains(errOut.String(), "no") {
			t.Fatalf("%s stderr = %q", name, errOut.String())
		}
	}
}
