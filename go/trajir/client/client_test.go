package client_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
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

func TestExecToolLogsToolCallOnError(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "t-err", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	boom := errors.New("tool fail")
	tool := resume.Tool{
		Name:   "fetch",
		Effect: effects.READ_ONLY,
		Fn:     func(args map[string]any) (any, error) { return nil, boom },
	}
	_, err = tr.ExecTool(1, 2, tool, map[string]any{"url": "https://example.com"})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v want %v", err, boom)
	}
	ok, err := tr.Log().Has("t-err", "demo", 1, "TOOL_CALL", intPtr(2))
	if err != nil || !ok {
		t.Fatalf("TOOL_CALL has=%v err=%v", ok, err)
	}
	hasResult, err := tr.Log().Has("t-err", "demo", 1, "TOOL_RESULT", intPtr(3))
	if err != nil || hasResult {
		t.Fatalf("expected no TOOL_RESULT on error, got %v (err: %v)", hasResult, err)
	}
}

func TestResumeTenantIsolation(t *testing.T) {
	dir := t.TempDir()
	opts := client.Options{WorkDir: dir}

	trA, err := client.OpenTrajectory("tenant-A", "shared-traj", opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trA.Project(1, map[string]any{"goal": "x"}); err != nil {
		t.Fatal(err)
	}
	_ = trA.Close()

	_, err = client.Resume("tenant-B", "shared-traj", opts)
	if err == nil {
		t.Fatal("expected Resume to fail for a different tenant, but it succeeded")
	}
	if !strings.Contains(err.Error(), "no existing nodes") {
		t.Fatalf("expected 'no existing nodes' error, got: %v", err)
	}
}

func TestSealHashIgnoresSink(t *testing.T) {
	plan := map[string]any{"tool_calls": []any{map[string]any{"name": "echo", "args": map[string]any{}}}}
	off := decisionID(t, nil, plan)
	on := decisionID(t, emit.SinkFunc(func(emit.Event) error {
		return errors.New("sink down")
	}), plan)
	panicID := decisionID(t, emit.SinkFunc(func(emit.Event) error {
		panic("sink panic")
	}), plan)
	if off == "" || off != on || off != panicID {
		t.Fatalf("decision ids off=%s on=%s panic=%s", off, on, panicID)
	}
}

func decisionID(t *testing.T, sink emit.Sink, plan map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "same-traj", client.Options{WorkDir: dir, ConsoleSink: sink})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if _, err := tr.Project(1, map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.SealDecision(1, plan); err != nil {
		t.Fatal(err)
	}
	rows, err := tr.Log().ListNodes("same-traj", "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row["kind"] == "DECISION" {
			id, _ := row["id"].(string)
			return id
		}
	}
	t.Fatal("missing DECISION")
	return ""
}

func TestExecToolRefusesOpenWorldTaggedReadOnly(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "ow-1", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	tool := resume.Tool{
		Name:   "bash",
		Effect: effects.READ_ONLY,
		Fn:     func(args map[string]any) (any, error) { return args["cmd"], nil },
	}
	_, err = tr.ExecTool(1, 2, tool, map[string]any{"cmd": "cat f"})
	var ow *effects.OpenWorldOverrideRequired
	if !errors.As(err, &ow) {
		t.Fatalf("err=%v want OpenWorldOverrideRequired", err)
	}
}

func TestExecToolAllowsOpenWorldWithOverride(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "ow-2", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	tool := resume.Tool{
		Name:                   "bash",
		Effect:                 effects.READ_ONLY,
		AllowOpenWorldOverride: true,
		Fn:                     func(args map[string]any) (any, error) { return args["cmd"], nil },
	}
	res, err := tr.ExecTool(1, 2, tool, map[string]any{"cmd": "cat f"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Result != "cat f" {
		t.Fatalf("result=%v", res.Result)
	}
}

func TestSealDecisionWorldSnapshotAndCheckWorld(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "w1", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if _, err := tr.Project(1, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.SealDecision(1, map[string]any{"tool_calls": []any{}}, client.SealDecisionOpts{
		WorldSnapshot: map[string]string{"cluster_generation": "42"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tr.CheckWorld(1, map[string]string{"cluster_generation": "42"}); err != nil {
		t.Fatal(err)
	}
	err = tr.CheckWorld(1, map[string]string{"cluster_generation": "43"})
	var drift *resume.WorldDrift
	if !errors.As(err, &drift) {
		t.Fatalf("err=%v want WorldDrift", err)
	}
}

func TestExecToolFnWithMetaSeesHashedKey(t *testing.T) {
	dir := t.TempDir()
	tr, err := client.OpenTrajectory("demo", "meta-1", client.Options{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	var seen resume.CallMeta
	tool := resume.Tool{
		Name:   "charge",
		Effect: effects.NON_IDEMPOTENT_WRITE,
		FnWithMeta: func(args map[string]any, meta resume.CallMeta) (any, error) {
			seen = meta
			if _, ok := args["idempotency_key"]; ok {
				t.Fatal("key must not be in args")
			}
			return "ok", nil
		},
	}
	if _, err := tr.ExecTool(1, 2, tool, map[string]any{"amount": 10}); err != nil {
		t.Fatal(err)
	}
	want := resume.IdempotencyKey("demo", "meta-1", 1, 2)
	if seen.IdempotencyKey != want {
		t.Fatalf("key=%q want %q", seen.IdempotencyKey, want)
	}
	h := resume.IdempotencyKeyHeader(seen.IdempotencyKey)
	if h["Idempotency-Key"] != want {
		t.Fatalf("header=%v", h)
	}
}
