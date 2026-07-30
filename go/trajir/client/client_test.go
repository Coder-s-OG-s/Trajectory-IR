package client_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

func TestOpenProjectSealCommit(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "t1", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	if _, err := tr.Project(1, map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.SealDecision(1, map[string]any{
		"tool_calls": []any{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tr.CommitStep(1, 2); err != nil {
		t.Fatal(err)
	}

	ok, err := tr.Log().Has("t1", 1, "DECISION", nil)
	if err != nil || !ok {
		t.Fatalf("DECISION present=%v err=%v", ok, err)
	}
	ok, err = tr.Log().Has("t1", 1, "COMMIT_STEP", nil)
	if err != nil || !ok {
		t.Fatalf("COMMIT_STEP present=%v err=%v", ok, err)
	}
}

func TestRunStepAndResumeNoSecondModelCall(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	opts := client.Options{WorkDir: dir, WorkflowID: "wf-client"}

	var modelCalls atomic.Int32
	tools := map[string]resume.Tool{
		"echo": {
			Name:   "echo",
			Effect: effects.PURE,
			Fn:     func(args map[string]any) (any, error) { return args["msg"], nil },
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		modelCalls.Add(1)
		return map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "echo", "args": map[string]any{"msg": "hi"}},
			},
		}, nil
	}

	tr, err := client.OpenTrajectory("demo", "t1", opts)
	if err != nil {
		t.Fatal(err)
	}
	results, err := tr.RunStep(ctx, 1, model, tools, map[string]any{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != "hi" {
		t.Fatalf("results=%#v", results)
	}
	_ = tr.Close()

	tr2, err := client.Resume("demo", "t1", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer tr2.Close()
	if _, err := tr2.RunStep(ctx, 1, model, tools, map[string]any{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if modelCalls.Load() != 1 {
		t.Fatalf("model calls=%d, want 1", modelCalls.Load())
	}
}

func TestExecToolGated(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "t1", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	var n atomic.Int32
	tool := resume.Tool{
		Name:   "deploy_server",
		Effect: effects.NON_IDEMPOTENT_WRITE,
		Fn: func(map[string]any) (any, error) {
			n.Add(1)
			return "ok", nil
		},
	}
	if _, err := tr.ExecTool(1, 2, tool, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	_, err = tr.ExecTool(1, 2, tool, map[string]any{})
	if err == nil {
		t.Fatal("expected block on second exec")
	}
	if n.Load() != 1 {
		t.Fatalf("side effects=%d", n.Load())
	}
}
