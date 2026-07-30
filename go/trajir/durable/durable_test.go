package durable_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
)

func TestMemoryStepMemoizes(t *testing.T) {
	ctx := context.Background()
	b := durable.NewMemory()
	t.Cleanup(func() { _ = b.Close() })

	var calls atomic.Int32
	fn := func(context.Context) (any, error) {
		calls.Add(1)
		return map[string]any{"n": float64(1)}, nil
	}

	v1, err := durable.Step(ctx, b, "wf1", "infer:model", fn)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := durable.Step(ctx, b, "wf1", "infer:model", fn)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("fn called %d times, want 1", calls.Load())
	}
	m1 := v1.(map[string]any)
	m2 := v2.(map[string]any)
	if m1["n"] != m2["n"] {
		t.Fatalf("results differ: %#v vs %#v", v1, v2)
	}
}

func TestLocalSQLiteSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memo.sqlite")

	var calls atomic.Int32
	fn := func(context.Context) (any, error) {
		calls.Add(1)
		return map[string]any{"plan": "seal-me"}, nil
	}

	b1, err := durable.OpenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := durable.Infer(ctx, b1, "run-1", "model", fn); err != nil {
		t.Fatal(err)
	}
	_ = b1.Close()

	b2, err := durable.OpenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b2.Close() })

	v, err := durable.Infer(ctx, b2, "run-1", "model", fn)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("after reopen fn called %d times, want 1", calls.Load())
	}
	m := v.(map[string]any)
	if m["plan"] != "seal-me" {
		t.Fatalf("unexpected memo: %#v", v)
	}
}

func TestToolPrefixIndependentOfInfer(t *testing.T) {
	ctx := context.Background()
	b := durable.NewMemory()
	t.Cleanup(func() { _ = b.Close() })

	var inferCalls, toolCalls atomic.Int32
	if _, err := durable.Infer(ctx, b, "wf", "m", func(context.Context) (any, error) {
		inferCalls.Add(1)
		return "plan", nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := durable.Tool(ctx, b, "wf", "echo", func(context.Context) (any, error) {
		toolCalls.Add(1)
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := durable.Infer(ctx, b, "wf", "m", func(context.Context) (any, error) {
		inferCalls.Add(1)
		return "plan", nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := durable.Tool(ctx, b, "wf", "echo", func(context.Context) (any, error) {
		toolCalls.Add(1)
		return "ok", nil
	}); err != nil {
		t.Fatal(err)
	}
	if inferCalls.Load() != 1 || toolCalls.Load() != 1 {
		t.Fatalf("infer=%d tool=%d, want 1 each", inferCalls.Load(), toolCalls.Load())
	}
}

func TestFirstSaveWins(t *testing.T) {
	ctx := context.Background()
	b := durable.NewMemory()
	t.Cleanup(func() { _ = b.Close() })

	if err := b.Save(ctx, "wf", "k", []byte(`"first"`)); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(ctx, "wf", "k", []byte(`"second"`)); err != nil {
		t.Fatal(err)
	}
	raw, found, err := b.Load(ctx, "wf", "k")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if string(raw) != `"first"` {
		t.Fatalf("got %s, want first", raw)
	}
}
