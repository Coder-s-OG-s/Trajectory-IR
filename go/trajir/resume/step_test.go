package resume_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
)

func testEnv(t *testing.T) (*nodelog.NodeLog, *durable.Memory) {
	t.Helper()
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	mem := durable.NewMemory()
	t.Cleanup(func() { _ = mem.Close() })
	return nl, mem
}

func TestRunStepHappyPathIdempotentTool(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)

	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     "demo",
		TrajectoryID: "t1",
		WorkflowID:   "wf1",
		Tools: map[string]resume.Tool{
			"echo": {
				Name:   "echo",
				Effect: effects.PURE,
				Fn: func(args map[string]any) (any, error) {
					return args["msg"], nil
				},
			},
		},
	}

	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{
				map[string]any{
					"name": "echo",
					"args": map[string]any{"msg": "hello"},
				},
			},
		}, nil
	}

	results, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != "hello" {
		t.Fatalf("results=%#v", results)
	}

	ok, _ := nl.Has("t1", "demo", 1, "DECISION", nil)
	if !ok {
		t.Fatal("missing DECISION")
	}
	ok, _ = nl.Has("t1", "demo", 1, "COMMIT_STEP", nil)
	if !ok {
		t.Fatal("missing COMMIT_STEP")
	}
}

func TestRunStepModelNotReinvokedOnReplay(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	var modelCalls atomic.Int32

	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     "demo",
		TrajectoryID: "t1",
		WorkflowID:   "wf-replay",
		Tools: map[string]resume.Tool{
			"echo": {
				Name:   "echo",
				Effect: effects.PURE,
				Fn:     func(args map[string]any) (any, error) { return "ok", nil },
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		modelCalls.Add(1)
		return map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "echo", "args": map[string]any{}},
			},
		}, nil
	}

	if _, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if modelCalls.Load() != 1 {
		t.Fatalf("model calls=%d, want 1", modelCalls.Load())
	}
}

func TestRunStepNonIdempotentGateOnReentry(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	var deploys atomic.Int32

	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     "demo",
		TrajectoryID: "t1",
		WorkflowID:   "wf-gate",
		Tools: map[string]resume.Tool{
			"deploy_server": {
				Name:   "deploy_server",
				Effect: effects.NON_IDEMPOTENT_WRITE,
				Fn: func(map[string]any) (any, error) {
					deploys.Add(1)
					return "deployed", nil
				},
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "deploy_server", "args": map[string]any{"x": "1"}},
			},
		}, nil
	}

	if _, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if deploys.Load() != 1 {
		t.Fatalf("deploys=%d after first run", deploys.Load())
	}

	// Same workflow id: durable.Tool returns memoized result without calling the
	// gated body again. Side effect must stay 1. Gate would also block if the
	// durable layer re-entered the body after TOOL_CALL was logged.
	if _, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if deploys.Load() != 1 {
		t.Fatalf("deploys=%d after second run, want 1", deploys.Load())
	}
}

func TestRunStepGateWhenToolCallAlreadyLogged(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	var deploys atomic.Int32

	// Simulate crash after TOOL_CALL was written but before durable memo finished.
	step := 1
	seq := 2
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{"tool": "deploy_server", "args": map[string]any{}}, "t1", "demo", seq); err != nil {
		t.Fatal(err)
	}

	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     "demo",
		TrajectoryID: "t1",
		WorkflowID:   "wf-partial",
		Tools: map[string]resume.Tool{
			"deploy_server": {
				Name:   "deploy_server",
				Effect: effects.NON_IDEMPOTENT_WRITE,
				Fn: func(map[string]any) (any, error) {
					deploys.Add(1)
					return "deployed", nil
				},
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "deploy_server", "args": map[string]any{}},
			},
		}, nil
	}

	_, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{})
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("want BlockedNeedsGate, got %v", err)
	}
	if deploys.Load() != 0 {
		t.Fatalf("deploy ran %d times, want 0", deploys.Load())
	}
}

func TestRunStepRequiresLogAndBackend(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{"tool_calls": []any{}}, nil
	}

	if _, err := resume.RunStep(ctx, resume.RunStepConfig{Backend: backend, TrajectoryID: "t1"}, 1, model, nil); err == nil {
		t.Fatal("expected error for nil Log")
	}
	if _, err := resume.RunStep(ctx, resume.RunStepConfig{Log: nl, TrajectoryID: "t1"}, 1, model, nil); err == nil {
		t.Fatal("expected error for nil Backend")
	}
}

func TestRunStepPropagatesModelError(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	boom := errors.New("model boom")
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return nil, boom
	}
	cfg := resume.RunStepConfig{Log: nl, Backend: backend, TrajectoryID: "t1", TenantID: "demo"}
	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); !errors.Is(err, boom) {
		t.Fatalf("err=%v want %v", err, boom)
	}
}

func TestRunStepMissingToolCallsInPlan(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{}, nil
	}
	cfg := resume.RunStepConfig{Log: nl, Backend: backend, TrajectoryID: "t1", TenantID: "demo"}
	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); err == nil {
		t.Fatal("expected error for plan missing tool_calls")
	}
}

func TestRunStepUnknownTool(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{map[string]any{"name": "ghost"}},
		}, nil
	}
	cfg := resume.RunStepConfig{Log: nl, Backend: backend, TrajectoryID: "t1", TenantID: "demo", Tools: map[string]resume.Tool{}}
	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRunStepToolCallMissingName(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{map[string]any{"args": map[string]any{}}},
		}, nil
	}
	cfg := resume.RunStepConfig{Log: nl, Backend: backend, TrajectoryID: "t1", TenantID: "demo"}
	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); err == nil {
		t.Fatal("expected error for tool_call missing name")
	}
}

func TestRunStepSandboxModeBlocksNonIdempotentWrite(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	var deploys atomic.Int32
	cfg := resume.RunStepConfig{
		Log: nl, Backend: backend, TrajectoryID: "t1", TenantID: "demo",
		Mode: sandbox.ModeSandbox,
		Tools: map[string]resume.Tool{
			"deploy_server": {
				Name:   "deploy_server",
				Effect: effects.NON_IDEMPOTENT_WRITE,
				Fn: func(map[string]any) (any, error) {
					deploys.Add(1)
					return "deployed", nil
				},
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{map[string]any{"name": "deploy_server"}},
		}, nil
	}
	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); err == nil {
		t.Fatal("expected sandbox rejection")
	}
	if deploys.Load() != 0 {
		t.Fatalf("deploys=%d, want 0", deploys.Load())
	}
}

func TestRunStepNonGatedToolErrorLogsToolCall(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	boom := errors.New("tool boom")
	cfg := resume.RunStepConfig{
		Log: nl, Backend: backend, TrajectoryID: "t1", TenantID: "demo",
		Tools: map[string]resume.Tool{
			"echo": {
				Name:   "echo",
				Effect: effects.PURE,
				Fn:     func(map[string]any) (any, error) { return nil, boom },
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{map[string]any{"name": "echo"}},
		}, nil
	}
	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); !errors.Is(err, boom) {
		t.Fatalf("err=%v want %v", err, boom)
	}
	ok, err := nl.Has("t1", "demo", 1, "TOOL_CALL", intPtr(2))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("TOOL_CALL must be logged when non-gated tool errors")
	}
	hasResult, err := nl.Has("t1", "demo", 1, "TOOL_RESULT", intPtr(3))
	if err != nil || hasResult {
		t.Fatalf("expected no TOOL_RESULT on error, got %v (err: %v)", hasResult, err)
	}
}

func TestRunStepReplayDoesNotDuplicateToolNodes(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	var executions atomic.Int32

	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     "demo",
		TrajectoryID: "t1",
		WorkflowID:   "wf-replay-nodup",
		Tools: map[string]resume.Tool{
			"echo": {
				Name:   "echo",
				Effect: effects.PURE,
				Fn: func(args map[string]any) (any, error) {
					executions.Add(1)
					return args["msg"], nil
				},
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "echo", "args": map[string]any{"msg": "hi"}},
			},
		}, nil
	}

	if _, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if executions.Load() != 1 {
		t.Fatalf("executions=%d want 1", executions.Load())
	}

	// Replay the step under the same workflow ID.
	if _, err := resume.RunStep(ctx, cfg, 1, model, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if executions.Load() != 1 {
		t.Fatalf("executions after replay=%d want 1", executions.Load())
	}

	nodes, err := nl.ListNodes("t1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	// Expected exactly: PROJECT_CONTEXT, DECISION, TOOL_CALL, TOOL_RESULT, COMMIT_STEP
	if len(nodes) != 5 {
		t.Fatalf("expected 5 nodes after replay, got %d", len(nodes))
	}
}

func TestRunStepReplayAfterToolFailureDoesNotConflict(t *testing.T) {
	ctx := context.Background()
	nl, backend := testEnv(t)
	var calls atomic.Int32
	boom1 := errors.New("timeout after 100ms")
	boom2 := errors.New("timeout after 150ms")

	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     "demo",
		TrajectoryID: "t1",
		WorkflowID:   "wf-fail-replay",
		Tools: map[string]resume.Tool{
			"failing": {
				Name:   "failing",
				Effect: effects.PURE,
				Fn: func(map[string]any) (any, error) {
					c := calls.Add(1)
					if c == 1 {
						return nil, boom1
					}
					return nil, boom2
				},
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		return map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "failing", "args": map[string]any{}},
			},
		}, nil
	}

	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); !errors.Is(err, boom1) {
		t.Fatalf("expected boom1, got %v", err)
	}

	// Retry with differently-worded error: must surface boom2 without slot conflict.
	if _, err := resume.RunStep(ctx, cfg, 1, model, nil); !errors.Is(err, boom2) {
		t.Fatalf("expected boom2 on replay, got %v", err)
	}

	nodes, err := nl.ListNodes("t1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	// PROJECT_CONTEXT, DECISION, TOOL_CALL (no TOOL_RESULT on error)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
}
