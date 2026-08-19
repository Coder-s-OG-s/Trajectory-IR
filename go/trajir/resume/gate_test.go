package resume_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

func openLog(t *testing.T) *nodelog.NodeLog {
	t.Helper()
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	return nl
}

func TestGatedToolRunsOnceThenBlocks(t *testing.T) {
	nl := openLog(t)
	sideEffects := 0
	tool := func(args map[string]any) (any, error) {
		sideEffects++
		return map[string]any{"ok": true, "n": args["n"]}, nil
	}

	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy_server", tool)

	out, err := gated(map[string]any{"n": float64(1)})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if sideEffects != 1 {
		t.Fatalf("sideEffects=%d after first call", sideEffects)
	}
	m := out.(map[string]any)
	if m["ok"] != true {
		t.Fatalf("unexpected result: %#v", out)
	}

	_, err = gated(map[string]any{"n": float64(1)})
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("second call err=%v, want BlockedNeedsGate", err)
	}
	if blocked.StepN != 1 || blocked.ToolName != "deploy_server" {
		t.Fatalf("blocked fields: %+v", blocked)
	}
	if sideEffects != 1 {
		t.Fatalf("sideEffects=%d after block, want 1", sideEffects)
	}

	ok, err := nl.Has("t1", "demo", 1, "TOOL_CALL", intPtr(2))
	if err != nil || !ok {
		t.Fatalf("TOOL_CALL present=%v err=%v", ok, err)
	}
	ok, err = nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || !ok {
		t.Fatalf("ABORT present=%v err=%v", ok, err)
	}
}

func TestGateKeysOnSeqNotSiblingTool(t *testing.T) {
	nl := openLog(t)
	callsA, callsB := 0, 0

	a := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "tool_a", func(map[string]any) (any, error) {
		callsA++
		return "a", nil
	})
	b := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 4, "tool_b", func(map[string]any) (any, error) {
		callsB++
		return "b", nil
	})

	if _, err := a(nil); err != nil {
		t.Fatal(err)
	}
	// Completing A must not block first entry of B at a different seq.
	if _, err := b(nil); err != nil {
		t.Fatal(err)
	}
	if callsA != 1 || callsB != 1 {
		t.Fatalf("calls A=%d B=%d", callsA, callsB)
	}

	// Re-entry of A still blocks.
	_, err := a(nil)
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("want block on A re-entry, got %v", err)
	}
	if callsA != 1 {
		t.Fatalf("A re-ran: callsA=%d", callsA)
	}
}

func TestGatedToolPropagatesToolError(t *testing.T) {
	nl := openLog(t)
	boom := errors.New("boom")
	tool := func(map[string]any) (any, error) {
		return nil, boom
	}
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "failing_tool", tool)

	_, err := gated(nil)
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v want %v", err, boom)
	}
	ok, err := nl.Has("t1", "demo", 1, "TOOL_RESULT", intPtr(3))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("TOOL_RESULT should not be appended when tool errors")
	}
}

func TestErrorTextIncludesStepAndTool(t *testing.T) {
	e := &resume.BlockedNeedsGate{StepN: 3, ToolName: "deploy_server"}
	msg := e.Error()
	if msg == "" {
		t.Fatal("empty error text")
	}
	if !strings.Contains(msg, "3") || !strings.Contains(msg, "deploy_server") || !strings.Contains(msg, "BLOCKED_NEEDS_GATE") {
		t.Fatalf("unexpected message: %s", msg)
	}
}

// TestR03PureRecomputeNotBlocked locks R03: PURE tools are not claim-gated.
// A prior TOOL_CALL at the same seq would block under MakeGatedToolCall (R02),
// but the non-gated path may call the body freely and recompute.
func TestR03PureRecomputeNotBlocked(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append(
		"TOOL_CALL",
		&step,
		map[string]any{"tool": "compute", "args": map[string]any{}},
		"t1",
		"demo",
		2,
	); err != nil {
		t.Fatal(err)
	}

	calls := 0
	pure := func(map[string]any) (any, error) {
		calls++
		return calls, nil
	}

	// Non-gated path (RunStep else branch for PURE): recompute freely.
	if _, err := pure(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pure(nil); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}

	// Same history under the gate would block (R02 dual).
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "compute", pure)
	_, err := gated(nil)
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("want BlockedNeedsGate, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("gate re-ran body: calls=%d", calls)
	}
}

func intPtr(v int) *int { return &v }
