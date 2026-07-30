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

	ok, _ := nl.Has("t1", 1, "DECISION", nil)
	if !ok {
		t.Fatal("missing DECISION")
	}
	ok, _ = nl.Has("t1", 1, "COMMIT_STEP", nil)
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
