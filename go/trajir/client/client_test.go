package client_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
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

	ok, err := tr.Log().Has("t1", "demo", 1, "DECISION", nil)
	if err != nil || !ok {
		t.Fatalf("DECISION present=%v err=%v", ok, err)
	}
	ok, err = tr.Log().Has("t1", "demo", 1, "COMMIT_STEP", nil)
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

func TestResumeRequiresHistory(t *testing.T) {
	dir := t.TempDir()
	opts := client.Options{WorkDir: dir}
	_, err := client.Resume("demo", "brand-new", opts)
	if err == nil {
		t.Fatal("expected error resuming empty trajectory")
	}
	if !strings.Contains(err.Error(), "no existing nodes") {
		t.Fatalf("err=%v", err)
	}

	tr, err := client.OpenTrajectory("demo", "t-resume", opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Project(1, map[string]any{"goal": "x"}); err != nil {
		t.Fatal(err)
	}
	_ = tr.Close()

	tr2, err := client.Resume("demo", "t-resume", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer tr2.Close()
	if tr2.TrajectoryID != "t-resume" || tr2.TenantID != "demo" {
		t.Fatalf("ids=%s/%s", tr2.TrajectoryID, tr2.TenantID)
	}
}

func TestExecToolLogsPlainTools(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "t-plain", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	tool := resume.Tool{
		Name:   "echo",
		Effect: effects.PURE,
		Fn:     func(args map[string]any) (any, error) { return args["msg"], nil },
	}
	res, err := tr.ExecTool(1, 2, tool, map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != "hi" {
		t.Fatalf("result=%v", res.Result)
	}
	ok, err := tr.Log().Has("t-plain", "demo", 1, "TOOL_CALL", intPtr(2))
	if err != nil || !ok {
		t.Fatalf("TOOL_CALL has=%v err=%v", ok, err)
	}
	ok, err = tr.Log().Has("t-plain", "demo", 1, "TOOL_RESULT", intPtr(3))
	if err != nil || !ok {
		t.Fatalf("TOOL_RESULT has=%v err=%v", ok, err)
	}
}

func intPtr(v int) *int { return &v }

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

func TestSandboxRejectsNonIdempotent(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "t-sb", client.Options{
		WorkDir: dir,
		Mode:    sandbox.ModeSandbox,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	tool := resume.Tool{
		Name:   "deploy",
		Effect: effects.NON_IDEMPOTENT_WRITE,
		Fn:     func(map[string]any) (any, error) { return "nope", nil },
	}
	if _, err := tr.ExecTool(1, 2, tool, nil); err == nil {
		t.Fatal("expected sandbox reject")
	}
}
