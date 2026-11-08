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
	// No ABORT yet: the child may still finish, and one slot has one outcome.
	ok, err := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none", ok, err)
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

func TestGatedAbortIsFinalAndResolveIsNotAsked(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")
	body := func(map[string]any) (any, error) {
		t.Fatal("spawn must not run again")
		return nil, nil
	}

	args := map[string]any{"task": "build"}
	var blocked *resume.BlockedNeedsGate
	if _, err := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "spawn_builder", body)(args); !errors.As(err, &blocked) {
		t.Fatalf("err=%v, want BlockedNeedsGate", err)
	}

	gated := resume.MakeGatedToolCallWithResolve(nl, "t1", "demo", 1, 2, "spawn_builder", body,
		func(map[string]any, resume.CallMeta) (any, bool, error) {
			t.Fatal("resolve must not be asked after ABORT")
			return nil, false, nil
		})
	if _, err := gated(args); !errors.As(err, &blocked) {
		t.Fatalf("err=%v, want BlockedNeedsGate", err)
	}
	ok, err := nl.Has("t1", "demo", 1, "TOOL_RESULT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("TOOL_RESULT present=%v err=%v, want none", ok, err)
	}
}

func TestGatedLosingResolveRaceReturnsWinnersResult(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")
	step := 1
	gated := resume.MakeGatedToolCallWithResolve(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) { return nil, nil },
		func(map[string]any, resume.CallMeta) (any, bool, error) {
			// Another worker commits its outcome while this lookup is in flight.
			if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{"result": "winner"}, "t1", "demo", 3); err != nil {
				t.Fatal(err)
			}
			return "loser", true, nil
		})
	got, err := gated(map[string]any{"task": "build"})
	if err != nil {
		t.Fatalf("losing the race must not fail the step: %v", err)
	}
	if got != "winner" {
		t.Fatalf("result=%v, want winner", got)
	}
}

func TestGatedLosingRaceToIntegerFailureReturnsIt(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")
	step := 1
	gated := resume.MakeGatedToolCallWithResolve(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) { return nil, nil },
		func(map[string]any, resume.CallMeta) (any, bool, error) {
			if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{
				"error":         "integer_domain",
				"message":       "too big",
				"result_digest": strings.Repeat("ab", 32),
			}, "t1", "demo", 3); err != nil {
				t.Fatal(err)
			}
			return "loser", true, nil
		})
	_, err := gated(map[string]any{"task": "build"})
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) || unsafe.Message != "too big" {
		t.Fatalf("err=%v, want UnsafeInteger too big", err)
	}
}

func TestGatedRunningToolLosingToResolverReturnsRecordedResult(t *testing.T) {
	nl := openLog(t)
	step := 1
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) {
			if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{"result": "resolved"}, "t1", "demo", 3); err != nil {
				t.Fatal(err)
			}
			return "ran", nil
		})
	got, err := gated(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "resolved" {
		t.Fatalf("result=%v, want resolved", got)
	}
}

func TestGatedUnsafeResultRecordsIntegerFailure(t *testing.T) {
	nl := openLog(t)
	runs := 0
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "mint",
		func(map[string]any) (any, error) {
			runs++
			return map[string]any{
				"a":    map[string]any{"b": "x\ny"},
				"id":   int64(9007199254740993),
				"n":    []any{1, true, nil},
				"name": "\u00e9\U0001F600<&",
			}, nil
		})

	_, err := gated(nil)
	var unsafe *resume.UnsafeInteger
	if !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
	stepN := 1
	row, found, err := nl.GetNodePayload("t1", "demo", &stepN, 3, "TOOL_RESULT")
	if err != nil || !found {
		t.Fatalf("failure row found=%v err=%v", found, err)
	}
	if row["error"] != "integer_domain" || row["message"] != unsafe.Message {
		t.Fatalf("row=%#v", row)
	}
	if _, ok := row["result"]; ok {
		t.Fatalf("row must not carry the value: %#v", row)
	}
	// Same value through Python's _result_digest.
	const want = "b82a14d55b108e7ec89101447f43a06d7a4c89d16318a6a05116f0619ff195f2"
	if row["result_digest"] != want {
		t.Fatalf("result_digest=%v, want %s", row["result_digest"], want)
	}

	// Re-entry reads the row back: same error, no second run, no ABORT.
	_, err = gated(nil)
	if !errors.As(err, &unsafe) {
		t.Fatalf("re-entry err=%v, want UnsafeInteger", err)
	}
	if runs != 1 {
		t.Fatalf("runs=%d, want 1", runs)
	}
	ok, err := nl.Has("t1", "demo", 1, "ABORT", intPtr(3))
	if err != nil || ok {
		t.Fatalf("ABORT present=%v err=%v, want none", ok, err)
	}
}

func TestPlainUnsafeResultRecordsIntegerFailure(t *testing.T) {
	nl := openLog(t)
	runs := 0
	plain := resume.MakePlainToolCall(nl, "t1", "demo", 1, 2, "mint",
		func(map[string]any) (any, error) {
			runs++
			return int64(9007199254740993), nil
		})
	var unsafe *resume.UnsafeInteger
	for i := 0; i < 2; i++ {
		if _, err := plain(nil); !errors.As(err, &unsafe) {
			t.Fatalf("call %d err=%v, want UnsafeInteger", i, err)
		}
	}
	if runs != 1 {
		t.Fatalf("runs=%d, want 1", runs)
	}
}

func TestGatedExistingAbortBlocksWithoutSecondAbortWrite(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")
	step := 1
	// A host wrote its own ABORT here; another ABORT payload would hit the slot.
	host := map[string]any{"reason": "HOST_FAILURE", "tool": "spawn_builder"}
	if _, err := nl.Append("ABORT", &step, host, "t1", "demo", 3); err != nil {
		t.Fatal(err)
	}
	body := func(map[string]any) (any, error) {
		t.Fatal("tool must not run")
		return nil, nil
	}
	resolve := func(map[string]any, resume.CallMeta) (any, bool, error) {
		t.Fatal("resolve must not be asked")
		return nil, false, nil
	}
	args := map[string]any{"task": "build"}
	for _, r := range []resume.ResolveFunc{nil, resolve} {
		gated := resume.MakeGatedToolCallWithResolve(nl, "t1", "demo", 1, 2, "spawn_builder", body, r)
		var blocked *resume.BlockedNeedsGate
		if _, err := gated(args); !errors.As(err, &blocked) {
			t.Fatalf("err=%v, want BlockedNeedsGate", err)
		}
	}
	got, _, err := nl.GetNodePayload("t1", "demo", &step, 3, "ABORT")
	if err != nil || got["reason"] != "HOST_FAILURE" {
		t.Fatalf("ABORT=%#v err=%v", got, err)
	}
}

func appendFailureRow(t *testing.T, nl *nodelog.NodeLog) {
	t.Helper()
	step := 1
	if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{
		"error":         "integer_domain",
		"message":       "too big",
		"result_digest": strings.Repeat("ab", 32),
	}, "t1", "demo", 3); err != nil {
		t.Fatal(err)
	}
}

func TestPlainDifferentArgsConflictBeforeIntegerFailure(t *testing.T) {
	nl := openLog(t)
	step := 1
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{
		"tool":            "mint",
		"args":            map[string]any{"n": 1},
		"idempotency_key": resume.IdempotencyKey("demo", "t1", 1, 2),
	}, "t1", "demo", 2); err != nil {
		t.Fatal(err)
	}
	appendFailureRow(t, nl)
	plain := resume.MakePlainToolCall(nl, "t1", "demo", 1, 2, "mint",
		func(map[string]any) (any, error) { return nil, nil })

	var conflict *resume.ToolCallConflict
	if _, err := plain(map[string]any{"n": 2}); !errors.As(err, &conflict) {
		t.Fatalf("err=%v, want ToolCallConflict", err)
	}
	var unsafe *resume.UnsafeInteger
	if _, err := plain(map[string]any{"n": 1}); !errors.As(err, &unsafe) {
		t.Fatalf("err=%v, want UnsafeInteger", err)
	}
}

func TestGatedDifferentArgsConflictBeforeIntegerFailure(t *testing.T) {
	nl := openLog(t)
	claimWithoutResult(t, nl, "spawn_builder")
	appendFailureRow(t, nl)
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "spawn_builder",
		func(map[string]any) (any, error) { return nil, nil })
	var conflict *resume.ToolCallConflict
	if _, err := gated(map[string]any{"task": "deploy"}); !errors.As(err, &conflict) {
		t.Fatalf("err=%v, want ToolCallConflict", err)
	}
}
