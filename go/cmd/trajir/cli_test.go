package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/console"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

type harness struct {
	t       testing.TB
	project string
	data    string
	env     map[string]string
}

func newHarness(t testing.TB) *harness {
	t.Helper()
	root := t.TempDir()
	h := &harness{t: t, project: filepath.Join(root, "proj"), data: filepath.Join(root, "console-data")}
	if err := os.MkdirAll(h.project, 0o755); err != nil {
		t.Fatal(err)
	}
	h.env = map[string]string{"TRAJIR_CONSOLE_DATA": h.data, "HOME": root, "USERPROFILE": root}
	return h
}

func (h *harness) run(stdin string, args ...string) (int, string, string) {
	h.t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, cliIO{
		stdin:  strings.NewReader(stdin),
		stdout: &out,
		stderr: &errOut,
		getenv: func(k string) string { return h.env[k] },
	})
	return code, out.String(), errOut.String()
}

// payload builds a hook body with cwd set to the harness project.
func (h *harness) payload(fields map[string]any) string {
	h.t.Helper()
	body := map[string]any{"cwd": h.project}
	for k, v := range fields {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	return string(raw)
}

func claudeEvent(h *harness, id, tool string, input map[string]any, extra map[string]any) string {
	f := map[string]any{"session_id": "abc123", "tool_use_id": id, "tool_name": tool, "tool_input": input}
	for k, v := range extra {
		f[k] = v
	}
	return h.payload(f)
}

func (h *harness) counts(host, traj string) map[string]int {
	h.t.Helper()
	nl, err := nodelog.Open(filepath.Join(h.project, ".trajir", "runs", host, "nodes.sqlite"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer nl.Close()
	c, err := nl.CountByKind(traj, "local")
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}

func (h *harness) mustRun(stdin string, args ...string) string {
	h.t.Helper()
	code, out, errOut := h.run(stdin, args...)
	if code != 0 {
		h.t.Fatalf("%v exit %d\nstdout: %s\nstderr: %s", args, code, out, errOut)
	}
	return out
}

func TestPreThenPostRecordsStep(t *testing.T) {
	h := newHarness(t)
	in := map[string]any{"command": "npm test"}
	if out := h.mustRun(claudeEvent(h, "t1", "Bash", in, nil), "hook", "pre", "--host", "claude"); out != "" {
		t.Fatalf("pre wrote stdout: %q", out)
	}
	h.mustRun(claudeEvent(h, "t1", "Bash", in, map[string]any{"tool_response": map[string]any{"stdout": "ok"}}), "hook", "post", "--host", "claude")
	c := h.counts("claude", "claude-abc123")
	if c["DECISION"] != 1 || c["TOOL_RESULT"] != 1 || c["COMMIT_STEP"] != 1 {
		t.Fatalf("counts = %v", c)
	}
	if _, err := os.Stat(filepath.Join(h.data, "trajectories", "claude-abc123.ndjson")); err != nil {
		t.Fatalf("no dashboard events: %v", err)
	}
}

func TestFailureHookWritesAbort(t *testing.T) {
	h := newHarness(t)
	h.mustRun(claudeEvent(h, "t1", "Bash", map[string]any{}, nil), "hook", "pre", "--host", "claude")
	h.mustRun(claudeEvent(h, "t1", "Bash", map[string]any{}, map[string]any{"error": "exit 1"}), "hook", "post-failure", "--host", "claude")
	if c := h.counts("claude", "claude-abc123"); c["ABORT"] != 1 {
		t.Fatalf("counts = %v", c)
	}
}

func TestReplayBlocks(t *testing.T) {
	h := newHarness(t)
	ev := claudeEvent(h, "t1", "Bash", map[string]any{"command": "deploy"}, nil)
	h.mustRun(ev, "hook", "pre", "--host", "claude")
	code, _, errOut := h.run(ev, "hook", "pre", "--host", "claude")
	if code != 2 || !strings.Contains(errOut, "Bash") {
		t.Fatalf("replay exit %d stderr %q", code, errOut)
	}
}

func TestPostWithoutSealExitsZero(t *testing.T) {
	h := newHarness(t)
	code, out, errOut := h.run(claudeEvent(h, "never", "Bash", map[string]any{}, map[string]any{"tool_response": "x"}), "hook", "post", "--host", "claude")
	if code != 0 || out != "" || !strings.Contains(errOut, "no sealed call") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

func TestUnwritableWorkDirBlocksWithHint(t *testing.T) {
	h := newHarness(t)
	blocker := filepath.Join(h.project, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.env["TRAJIR_WORK_DIR"] = filepath.Join(blocker, "runs")
	code, _, errOut := h.run(claudeEvent(h, "t1", "Bash", map[string]any{}, nil), "hook", "pre", "--host", "claude")
	if code != 2 || !strings.Contains(errOut, "TRAJIR_WORK_DIR") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestBadJSONBlocksPreOnly(t *testing.T) {
	h := newHarness(t)
	if code, _, _ := h.run("{nope", "hook", "pre", "--host", "claude"); code != 2 {
		t.Fatalf("pre exit %d", code)
	}
	if code, _, _ := h.run("{nope", "hook", "post", "--host", "claude"); code != 0 {
		t.Fatalf("post exit %d", code)
	}
}

func TestUnknownHostBlocksPreOnly(t *testing.T) {
	h := newHarness(t)
	if code, _, errOut := h.run("{}", "hook", "pre", "--host", "nope"); code != 2 || !strings.Contains(errOut, "unknown host") {
		t.Fatalf("pre exit %d %q", code, errOut)
	}
	if code, _, _ := h.run("{}", "hook", "post", "--host", "nope"); code != 0 {
		t.Fatalf("post exit %d", code)
	}
}

func TestUnknownPhaseExits2(t *testing.T) {
	h := newHarness(t)
	if code, _, _ := h.run("{}", "hook", "during", "--host", "claude"); code != 2 {
		t.Fatalf("exit %d", code)
	}
}

func TestEffectsFileErrorBlocksPre(t *testing.T) {
	h := newHarness(t)
	if err := os.MkdirAll(filepath.Join(h.project, ".trajir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.project, ".trajir", "effects.json"), []byte(`{"tools":{"x":"SAFE"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := h.run(claudeEvent(h, "t1", "x", map[string]any{}, nil), "hook", "pre", "--host", "claude")
	if code != 2 || !strings.Contains(errOut, "effects.json") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestLargeResultRecorded(t *testing.T) {
	h := newHarness(t)
	h.mustRun(claudeEvent(h, "t1", "Read", map[string]any{"file_path": "big"}, nil), "hook", "pre", "--host", "claude")
	big := strings.Repeat("x", 5<<20)
	start := time.Now()
	h.mustRun(claudeEvent(h, "t1", "Read", map[string]any{"file_path": "big"}, map[string]any{"tool_response": big}), "hook", "post", "--host", "claude")
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("5 MB result took %s", d)
	}
	if c := h.counts("claude", "claude-abc123"); c["TOOL_RESULT"] != 1 {
		t.Fatalf("counts = %v", c)
	}
}

// A payload over the limit says so. Cut short, it would fail as broken JSON.
func TestHookPayloadOverLimit(t *testing.T) {
	old := maxPayload
	maxPayload = 1 << 10
	t.Cleanup(func() { maxPayload = old })
	h := newHarness(t)
	big := map[string]any{"command": strings.Repeat("x", 2<<10)}
	code, _, errOut := h.run(claudeEvent(h, "t1", "Bash", big, nil), "hook", "pre", "--host", "claude")
	if code != 2 || !strings.Contains(errOut, "too large to record") {
		t.Fatalf("pre: exit %d %q", code, errOut)
	}
	code, _, errOut = h.run(claudeEvent(h, "t1", "Bash", big, map[string]any{"tool_response": "ok"}), "hook", "post", "--host", "claude")
	if code != 0 || !strings.Contains(errOut, "too large to record") {
		t.Fatalf("post: exit %d %q", code, errOut)
	}
}

func TestCopilotRoundTripWithoutIDs(t *testing.T) {
	h := newHarness(t)
	pre := h.payload(map[string]any{"sessionId": "cp1", "toolName": "bash", "toolArgs": `{"command":"make"}`})
	post := h.payload(map[string]any{"sessionId": "cp1", "toolName": "bash", "toolArgs": `{"command":"make"}`,
		"toolResult": map[string]any{"resultType": "success", "textResultForLlm": "ok"}})
	h.mustRun(pre, "hook", "pre", "--host", "copilot")
	h.mustRun(post, "hook", "post", "--host", "copilot")
	if c := h.counts("copilot", "copilot-cp1"); c["TOOL_RESULT"] != 1 || c["COMMIT_STEP"] != 1 {
		t.Fatalf("counts = %v", c)
	}
}

// A project and a user install of Gemini CLI or Copilot CLI both hook the
// same call. Neither sends a tool id, and the call must still be one step.
func TestTwoRegistrationsWithoutIDsRecordOnce(t *testing.T) {
	h := newHarness(t)
	pre := h.payload(map[string]any{"session_id": "g1", "tool_name": "run_shell_command", "tool_input": map[string]any{"command": "ls"}})
	post := h.payload(map[string]any{"session_id": "g1", "tool_name": "run_shell_command", "tool_input": map[string]any{"command": "ls"},
		"tool_response": map[string]any{"llmContent": "a.txt"}})
	for _, owner := range []string{"project", "user"} {
		h.mustRun(pre, "hook", "pre", "--host", "gemini", "--owner", owner)
	}
	for _, owner := range []string{"project", "user"} {
		if code, _, errOut := h.run(post, "hook", "post", "--host", "gemini", "--owner", owner); code != 0 || errOut != "" {
			t.Fatalf("%s post: exit %d %q", owner, code, errOut)
		}
	}
	if c := h.counts("gemini", "gemini-g1"); c["DECISION"] != 1 || c["TOOL_RESULT"] != 1 || c["COMMIT_STEP"] != 1 {
		t.Fatalf("counts = %v", c)
	}
}

func TestGeminiErrorResponseWritesAbort(t *testing.T) {
	h := newHarness(t)
	base := map[string]any{"session_id": "g1", "tool_name": "read_file", "tool_input": map[string]any{"absolute_path": "a"}}
	h.mustRun(h.payload(base), "hook", "pre", "--host", "gemini")
	base["tool_response"] = map[string]any{"llmContent": "", "error": "File not found"}
	h.mustRun(h.payload(base), "hook", "post", "--host", "gemini")
	if c := h.counts("gemini", "gemini-g1"); c["ABORT"] != 1 {
		t.Fatalf("counts = %v", c)
	}
}

func TestCursorBlockWritesDenyJSON(t *testing.T) {
	h := newHarness(t)
	ev := h.payload(map[string]any{"conversation_id": "c1", "tool_use_id": "u1", "tool_name": "Shell", "tool_input": map[string]any{"command": "rm"}})
	h.mustRun(ev, "hook", "pre", "--host", "cursor")
	code, out, _ := h.run(ev, "hook", "pre", "--host", "cursor")
	if code != 2 || !strings.Contains(out, `"permission":"deny"`) {
		t.Fatalf("exit %d stdout %q", code, out)
	}
}

func TestSessionStartIsQuietOnStdout(t *testing.T) {
	h := newHarness(t)
	code, out, errOut := h.run(h.payload(map[string]any{"session_id": "s"}), "hook", "session-start", "--host", "claude")
	if code != 0 || out != "" || !strings.Contains(errOut, "trajir console") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(h.project, ".trajir", ".gitignore")); err != nil {
		t.Fatalf("no .gitignore: %v", err)
	}
}

func TestExportVerifyShowOnDashboard(t *testing.T) {
	h := newHarness(t)
	h.mustRun(claudeEvent(h, "t1", "Bash", map[string]any{"command": "x"}, nil), "hook", "pre", "--host", "claude")
	h.mustRun(claudeEvent(h, "t1", "Bash", map[string]any{"command": "x"}, map[string]any{"tool_response": "ok"}), "hook", "post", "--host", "claude")
	pack := filepath.Join(h.project, "session.tir")
	h.mustRun("", "export", "--project", h.project, "--host", "claude", "--trajectory", "claude-abc123", "-o", pack)
	if out := h.mustRun("", "verify", pack); !strings.HasPrefix(out, "OK") {
		t.Fatalf("verify stdout %q", out)
	}
	store, err := console.OpenStore(h.data)
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.ReadEvents("claude-abc123")
	if err != nil {
		t.Fatal(err)
	}
	sum := console.Summarize("claude-abc123", events)
	if sum.ExportsOK != 1 || sum.Evidence.AuditOK == nil || !*sum.Evidence.AuditOK || sum.SealCreatedCount != 1 {
		t.Fatalf("summary exports=%d audit=%v seals=%d", sum.ExportsOK, sum.Evidence.AuditOK, sum.SealCreatedCount)
	}
	pkgs, err := filepath.Glob(filepath.Join(h.data, "packages", "claude-abc123", "*.tir"))
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("staged packages = %v, %v", pkgs, err)
	}
}

func TestExportUnknownTrajectoryExit1(t *testing.T) {
	h := newHarness(t)
	h.mustRun(claudeEvent(h, "t1", "Bash", map[string]any{}, nil), "hook", "pre", "--host", "claude")
	code, _, errOut := h.run("", "export", "--project", h.project, "--host", "claude", "--trajectory", "nope", "-o", filepath.Join(h.project, "x.tir"))
	if code != 1 || !strings.Contains(errOut, "nope") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestRunsListsSessions(t *testing.T) {
	h := newHarness(t)
	h.mustRun(claudeEvent(h, "t1", "Bash", map[string]any{}, nil), "hook", "pre", "--host", "claude")
	out := h.mustRun("", "runs", "--project", h.project)
	if !strings.Contains(out, "claude-abc123") || !strings.Contains(out, "claude") {
		t.Fatalf("runs output %q", out)
	}
}

func TestRunsEmptyProject(t *testing.T) {
	h := newHarness(t)
	out := h.mustRun("", "runs", "--project", h.project)
	if !strings.Contains(out, "no runs") {
		t.Fatalf("runs output %q", out)
	}
}

func TestVersionAndHelp(t *testing.T) {
	h := newHarness(t)
	if out := h.mustRun("", "version"); !strings.Contains(out, "trajir") {
		t.Fatalf("version %q", out)
	}
	_, _, errOut := h.run("", "help")
	for _, cmd := range []string{"hook", "export", "runs", "console", "mcp", "install", "verify"} {
		if !strings.Contains(errOut, cmd) {
			t.Fatalf("help misses %s: %q", cmd, errOut)
		}
	}
}

func TestInstallCommand(t *testing.T) {
	h := newHarness(t)
	out := h.mustRun("", "install", "codex", "--target", h.project, "--command", "trajir")
	if !strings.Contains(out, "hooks.json") || !strings.Contains(out, "/hooks") {
		t.Fatalf("install output %q", out)
	}
	if code, _, errOut := h.run("", "install", "nope", "--target", h.project); code != 1 || !strings.Contains(errOut, "unknown host") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if code, _, _ := h.run("", "install"); code != 2 {
		t.Fatalf("bare install exit %d", code)
	}
}

// The plugin and a project install can both be registered. Each passes its
// own --owner, so the second pre hook is a duplicate, not a replay.
func TestTwoRegistrationsDoNotBlock(t *testing.T) {
	h := newHarness(t)
	ev := claudeEvent(h, "t1", "Bash", map[string]any{"command": "deploy"}, nil)
	h.mustRun(ev, "hook", "pre", "--host", "claude", "--owner", "plugin")
	if code, _, errOut := h.run(ev, "hook", "pre", "--host", "claude", "--owner", "project"); code != 0 {
		t.Fatalf("second registration blocked: exit %d %s", code, errOut)
	}
	if code, _, _ := h.run(ev, "hook", "pre", "--host", "claude", "--owner", "plugin"); code != 2 {
		t.Fatalf("same owner repeat not blocked: exit %d", code)
	}
	post := claudeEvent(h, "t1", "Bash", map[string]any{"command": "deploy"}, map[string]any{"tool_response": "ok"})
	h.mustRun(post, "hook", "post", "--host", "claude", "--owner", "plugin")
	if code, _, errOut := h.run(post, "hook", "post", "--host", "claude", "--owner", "project"); code != 0 || errOut != "" {
		t.Fatalf("second post: exit %d %q", code, errOut)
	}
	if c := h.counts("claude", "claude-abc123"); c["TOOL_RESULT"] != 1 || c["DECISION"] != 1 {
		t.Fatalf("counts = %v", c)
	}
}
