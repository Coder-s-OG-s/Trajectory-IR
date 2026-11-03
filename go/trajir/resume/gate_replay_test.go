package resume_test

import (
	"errors"
	"strings"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

func TestGatedReplayWithDifferentArgsConflicts(t *testing.T) {
	nl := openLog(t)
	runs := 0
	tool := func(args map[string]any) (any, error) {
		runs++
		return map[string]any{"amount": args["amount"]}, nil
	}
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "charge", tool)
	if _, err := gated(map[string]any{"amount": 10}); err != nil {
		t.Fatal(err)
	}

	_, err := gated(map[string]any{"amount": 20})
	var conflict *resume.ToolCallConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("err=%v, want ToolCallConflict", err)
	}
	if !errors.Is(err, nodelog.ErrSlotConflict) {
		t.Fatalf("ToolCallConflict should match ErrSlotConflict: %v", err)
	}
	if conflict.StepN != 1 || conflict.Seq != 2 || conflict.ToolName != "charge" {
		t.Fatalf("conflict fields: %+v", conflict)
	}
	if runs != 1 {
		t.Fatalf("runs=%d, want 1", runs)
	}

	// Same args written as a float still match: the check is by RFC 8785 hash.
	got, err := gated(map[string]any{"amount": float64(10)})
	if err != nil {
		t.Fatalf("same args replay: %v", err)
	}
	if m := got.(map[string]any); m["amount"] == nil {
		t.Fatalf("replay result: %#v", got)
	}
}

func TestGatedReplayWithDifferentToolNameConflicts(t *testing.T) {
	nl := openLog(t)
	first := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "charge", func(map[string]any) (any, error) {
		return "charged", nil
	})
	if _, err := first(nil); err != nil {
		t.Fatal(err)
	}
	other := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "refund", func(map[string]any) (any, error) {
		t.Fatal("refund body must not run")
		return nil, nil
	})
	_, err := other(nil)
	var conflict *resume.ToolCallConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("err=%v, want ToolCallConflict", err)
	}
}

func appendIntegerFailure(t *testing.T, nl *nodelog.NodeLog, tool string) {
	t.Helper()
	step := 1
	if _, err := nl.Append(
		"TOOL_CALL", &step,
		map[string]any{
			"tool":            tool,
			"args":            map[string]any{},
			"idempotency_key": resume.IdempotencyKey("demo", "t1", 1, 2),
		},
		"t1", "demo", 2,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append(
		"TOOL_RESULT", &step,
		map[string]any{
			"error":         "integer_domain",
			"message":       "9007199254740993 exceeds safe integer domain",
			"result_digest": strings.Repeat("ab", 32),
		},
		"t1", "demo", 3,
	); err != nil {
		t.Fatal(err)
	}
}

func TestGatedReplayOfIntegerFailureDoesNotAbort(t *testing.T) {
	nl := openLog(t)
	appendIntegerFailure(t, nl, "deploy")
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy", func(map[string]any) (any, error) {
		t.Fatal("tool body must not run again")
		return nil, nil
	})

	_, err := gated(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
	if unsafe.Message != "9007199254740993 exceeds safe integer domain" {
		t.Fatalf("message=%q", unsafe.Message)
	}
	var blocked *resume.BlockedNeedsGate
	if errors.As(err, &blocked) {
		t.Fatal("integer failure must not be BlockedNeedsGate")
	}
	ok, err := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none (spec 1.9)", ok, err)
	}
}

func TestPlainReplayOfIntegerFailureDoesNotRerun(t *testing.T) {
	nl := openLog(t)
	appendIntegerFailure(t, nl, "lookup")
	plain := resume.MakePlainToolCall(nl, "t1", "demo", 1, 2, "lookup", func(map[string]any) (any, error) {
		t.Fatal("tool body must not run again")
		return nil, nil
	})
	_, err := plain(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
}

func claimWithoutResult(t *testing.T, nl *nodelog.NodeLog, tool string) {
	t.Helper()
	claimed, err := nl.ClaimToolCall(
		1,
		map[string]any{
			"tool":            tool,
			"args":            map[string]any{"task": "build"},
			"idempotency_key": resume.IdempotencyKey("demo", "t1", 1, 2),
		},
		"t1", "demo", 2,
	)
	if err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
}

func TestGatedResolveRecordsResultInsteadOfBlocking(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")

	var seenKey string
	resolve := func(args map[string]any, meta resume.CallMeta) (any, bool, error) {
		seenKey = meta.IdempotencyKey
		if args["task"] != "build" {
			t.Fatalf("resolve args=%#v", args)
		}
		return map[string]any{"child": "done"}, true, nil
	}
	gated := resume.MakeGatedToolCallWithResolve(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) {
			t.Fatal("spawn must not run again")
			return nil, nil
		}, resolve)

	got, err := gated(map[string]any{"task": "build"})
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	if m := got.(map[string]any); m["child"] != "done" {
		t.Fatalf("result=%#v", got)
	}
	if seenKey != resume.IdempotencyKey("demo", "t1", 1, 2) {
		t.Fatalf("resolve saw key %q", seenKey)
	}
	ok, err := nl.Has("t1", "demo", 1, "TOOL_RESULT", intPtr(3))
	if err != nil || !ok {
		t.Fatalf("TOOL_RESULT present=%v err=%v", ok, err)
	}
	ok, err = nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none", ok, err)
	}

	// The recorded result now answers re-entry without asking resolve.
	noResolve := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) { return nil, nil })
	again, err := noResolve(map[string]any{"task": "build"})
	if err != nil {
		t.Fatal(err)
	}
	if m := again.(map[string]any); m["child"] != "done" {
		t.Fatalf("replay=%#v", again)
	}
}

func TestGatedResolveUnresolvedBlocksThenResolvesLater(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")

	finished := false
	resolve := func(map[string]any, resume.CallMeta) (any, bool, error) {
		if !finished {
			return nil, false, nil
		}
		return "child-ok", true, nil
	}
	gated := resume.MakeGatedToolCallWithResolve(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) {
			t.Fatal("spawn must not run again")
			return nil, nil
		}, resolve)

	_, err := gated(map[string]any{"task": "build"})
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("err=%v, want BlockedNeedsGate", err)
	}

	finished = true
	got, err := gated(map[string]any{"task": "build"})
	if err != nil {
		t.Fatalf("after child finished: %v", err)
	}
	if got != "child-ok" {
		t.Fatalf("result=%v", got)
	}
}

func TestGatedResolveErrorPropagatesWithoutAbort(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")
	lookupDown := errors.New("status backend down")
	gated := resume.MakeGatedToolCallWithResolve(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) { return nil, nil },
		func(map[string]any, resume.CallMeta) (any, bool, error) { return nil, false, lookupDown })

	_, err := gated(map[string]any{"task": "build"})
	if !errors.Is(err, lookupDown) {
		t.Fatalf("err=%v, want %v", err, lookupDown)
	}
	ok, err := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none", ok, err)
	}
}
