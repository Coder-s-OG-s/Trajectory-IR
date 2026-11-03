package resume_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
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

func TestGatedToolRunsOnceThenReturnsRecordedResult(t *testing.T) {
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

	// Replay after the TOOL_RESULT is durable (crash before the backend memo):
	// same result back, no second side effect, no ABORT.
	again, err := gated(map[string]any{"n": float64(1)})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if am := again.(map[string]any); am["ok"] != true || fmt.Sprint(am["n"]) != "1" {
		t.Fatalf("replay result: %#v", again)
	}
	if sideEffects != 1 {
		t.Fatalf("sideEffects=%d after replay, want 1", sideEffects)
	}
	ok, err := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none", ok, err)
	}
}

func TestGatedToolBlocksWhenCallClaimedWithoutResult(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append(
		"TOOL_CALL", &step,
		map[string]any{"tool": "deploy_server", "args": map[string]any{}},
		"t1", "demo", 2,
	); err != nil {
		t.Fatal(err)
	}
	sideEffects := 0
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy_server",
		func(map[string]any) (any, error) {
			sideEffects++
			return nil, nil
		})

	_, err := gated(nil)
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("err=%v, want BlockedNeedsGate", err)
	}
	if blocked.StepN != 1 || blocked.ToolName != "deploy_server" {
		t.Fatalf("blocked fields: %+v", blocked)
	}
	if sideEffects != 0 {
		t.Fatalf("sideEffects=%d after block, want 0", sideEffects)
	}
	ok, err := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
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

	// Re-entry of A returns its own recorded result, not B's, and does not re-run.
	got, err := a(nil)
	if err != nil {
		t.Fatalf("A re-entry: %v", err)
	}
	if got != "a" {
		t.Fatalf("A re-entry result=%v, want a", got)
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

func TestGatedIntegerDomainIsNotAnAbort(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append(
		"TOOL_CALL", &step,
		map[string]any{"tool": "deploy_server", "args": map[string]any{}},
		"t1", "demo", 2,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append(
		"TOOL_RESULT", &step,
		map[string]any{
			"error":         "integer_domain",
			"message":       "9007199254740993 exceeds safe integer domain for JSON floats",
			"result_digest": "abc",
		},
		"t1", "demo", 3,
	); err != nil {
		t.Fatal(err)
	}

	sideEffects := 0
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy_server",
		func(map[string]any) (any, error) {
			sideEffects++
			return "ran", nil
		})

	_, err := gated(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
	want := "9007199254740993 exceeds safe integer domain for JSON floats"
	if unsafe.Error() != want {
		t.Fatalf("message=%q", unsafe.Error())
	}
	if sideEffects != 0 {
		t.Fatalf("sideEffects=%d, want 0", sideEffects)
	}
	ok, err := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none", ok, err)
	}
}

func TestGatedIntegerDomainEmptyMessageUsesDefault(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append(
		"TOOL_CALL", &step,
		map[string]any{"tool": "compute", "args": map[string]any{}},
		"t1", "demo", 2,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append(
		"TOOL_RESULT", &step,
		map[string]any{
			"error":         "integer_domain",
			"message":       "",
			"result_digest": "abc",
		},
		"t1", "demo", 3,
	); err != nil {
		t.Fatal(err)
	}

	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "compute",
		func(map[string]any) (any, error) {
			t.Fatal("tool body ran")
			return nil, nil
		})
	_, err := gated(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
	if unsafe.Error() != "integer exceeds safe integer domain for JSON floats" {
		t.Fatalf("message=%q", unsafe.Error())
	}
}

func TestGatedIntegerDomainWithoutDigestStillBlocks(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append(
		"TOOL_CALL", &step,
		map[string]any{"tool": "deploy_server", "args": map[string]any{}},
		"t1", "demo", 2,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append(
		"TOOL_RESULT", &step,
		map[string]any{"error": "integer_domain", "message": "too big"},
		"t1", "demo", 3,
	); err != nil {
		t.Fatal(err)
	}

	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy_server",
		func(map[string]any) (any, error) {
			t.Fatal("tool body ran")
			return nil, nil
		})
	_, err := gated(nil)
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("err=%v, want BlockedNeedsGate", err)
	}
	ok, hasErr := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if hasErr != nil || !ok {
		t.Fatalf("ABORT present=%v err=%v", ok, hasErr)
	}
}

func TestGatedIntegerDomainWithoutCallDoesNotRun(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append(
		"TOOL_RESULT", &step,
		map[string]any{
			"error":         "integer_domain",
			"message":       "too big",
			"result_digest": "abc",
		},
		"t1", "demo", 3,
	); err != nil {
		t.Fatal(err)
	}

	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy_server",
		func(map[string]any) (any, error) {
			t.Fatal("tool body ran")
			return nil, nil
		})
	_, err := gated(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
	if unsafe.Error() != "too big" {
		t.Fatalf("message=%q", unsafe.Error())
	}
	ok, hasErr := nl.Has("t1", "demo", 1, "TOOL_CALL", intPtr(2))
	if hasErr != nil || ok {
		t.Fatalf("TOOL_CALL present=%v err=%v, want none", ok, hasErr)
	}
}

func TestPlainIntegerDomainDoesNotRunOrRecordCall(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append(
		"TOOL_RESULT", &step,
		map[string]any{
			"error":         "integer_domain",
			"message":       "too big",
			"result_digest": "abc",
		},
		"t1", "demo", 3,
	); err != nil {
		t.Fatal(err)
	}

	plain := resume.MakePlainToolCall(nl, "t1", "demo", 1, 2, "compute",
		func(map[string]any) (any, error) {
			t.Fatal("tool body ran")
			return nil, nil
		})
	_, err := plain(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
	if unsafe.Error() != "too big" {
		t.Fatalf("message=%q", unsafe.Error())
	}
	ok, hasErr := nl.Has("t1", "demo", 1, "TOOL_CALL", intPtr(2))
	if hasErr != nil || ok {
		t.Fatalf("TOOL_CALL present=%v err=%v, want none", ok, hasErr)
	}
}

func TestGatedUnsafeResultIsStoredAndNotRerun(t *testing.T) {
	nl := openLog(t)
	calls := 0
	big := int64(1<<53) + 1
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "mint",
		func(map[string]any) (any, error) {
			calls++
			return map[string]any{"id": big}, nil
		})

	_, err := gated(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("first err=%v, want UnsafeInteger", err)
	}
	want := "9007199254740993 exceeds safe integer domain for JSON floats"
	if unsafe.Error() != want {
		t.Fatalf("message=%q", unsafe.Error())
	}
	if calls != 1 {
		t.Fatalf("calls=%d after first, want 1", calls)
	}

	_, err = gated(nil)
	if !errors.As(err, &unsafe) {
		t.Fatalf("second err=%v, want UnsafeInteger", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d after second, want 1", calls)
	}
	ok, hasErr := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if hasErr != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none", ok, hasErr)
	}

	payload := toolResultPayload(t, nl, 3)
	if payload["error"] != "integer_domain" || payload["message"] != want {
		t.Fatalf("payload=%v", payload)
	}
	raw, err := json.Marshal(map[string]any{"id": big})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if payload["result_digest"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest=%v", payload["result_digest"])
	}
	if _, err := nodes.PayloadHash(payload); err != nil {
		t.Fatalf("failure row is not hashable: %v", err)
	}
}

func TestPlainUnsafeResultIsStoredAndNotRerun(t *testing.T) {
	nl := openLog(t)
	calls := 0
	big := int64(1<<53) + 1
	plain := resume.MakePlainToolCall(nl, "t1", "demo", 1, 2, "mint",
		func(map[string]any) (any, error) {
			calls++
			return map[string]any{"id": big}, nil
		})

	_, err := plain(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("first err=%v, want UnsafeInteger", err)
	}
	_, err = plain(nil)
	if !errors.As(err, &unsafe) {
		t.Fatalf("second err=%v, want UnsafeInteger", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}
	rows, err := nl.ListNodes("t1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	toolCalls := 0
	for _, row := range rows {
		if row["kind"] == "TOOL_CALL" {
			toolCalls++
		}
	}
	if toolCalls != 1 {
		t.Fatalf("TOOL_CALL count=%d, want 1", toolCalls)
	}
	payload := toolResultPayload(t, nl, 3)
	if payload["error"] != "integer_domain" {
		t.Fatalf("payload=%v", payload)
	}
}

func toolResultPayload(t *testing.T, nl *nodelog.NodeLog, seq int) map[string]any {
	t.Helper()
	rows, err := nl.ListNodes("t1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row["kind"] != "TOOL_RESULT" || row["seq"] != seq {
			continue
		}
		payload, ok := row["payload"].(map[string]any)
		if !ok {
			t.Fatalf("payload type %T", row["payload"])
		}
		return payload
	}
	t.Fatalf("no TOOL_RESULT at seq %d", seq)
	return nil
}

func intPtr(v int) *int { return &v }
