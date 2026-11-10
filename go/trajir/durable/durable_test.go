package durable_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
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

func TestStepKeepsUnsafeIntegerDigits(t *testing.T) {
	ctx := context.Background()
	b := durable.NewMemory()
	t.Cleanup(func() { _ = b.Close() })

	const unsafe int64 = (1 << 53) + 1
	const safe int64 = (1 << 53) - 1
	fn := func(context.Context) (any, error) {
		return map[string]any{
			"n":        unsafe,
			"neg":      -unsafe,
			"ok":       safe,
			"neg_safe": -safe,
			"pow":       int64(1 << 53),
			"pow54":     int64(1 << 54),
			"big_float": float64(1e18),
			"x":         1.5,
			"nest":      []any{map[string]any{"id": unsafe}},
		}, nil
	}

	check := func(v any) {
		t.Helper()
		m := v.(map[string]any)
		n, ok := m["n"].(json.Number)
		if !ok || n.String() != "9007199254740993" {
			t.Fatalf("n=%#v (%T)", m["n"], m["n"])
		}
		neg, ok := m["neg"].(json.Number)
		if !ok || neg.String() != "-9007199254740993" {
			t.Fatalf("neg=%#v (%T)", m["neg"], m["neg"])
		}
		if m["ok"] != float64(safe) {
			t.Fatalf("ok=%#v (%T)", m["ok"], m["ok"])
		}
		if m["neg_safe"] != float64(-safe) {
			t.Fatalf("neg_safe=%#v (%T)", m["neg_safe"], m["neg_safe"])
		}
		pow, ok := m["pow"].(json.Number)
		if !ok || pow.String() != "9007199254740992" {
			t.Fatalf("pow=%#v (%T)", m["pow"], m["pow"])
		}
		pow54, ok := m["pow54"].(json.Number)
		if !ok || pow54.String() != "18014398509481984" {
			t.Fatalf("pow54=%#v (%T)", m["pow54"], m["pow54"])
		}
		if m["big_float"] != float64(1e18) {
			t.Fatalf("big_float=%#v (%T)", m["big_float"], m["big_float"])
		}
		if m["x"] != 1.5 {
			t.Fatalf("x=%#v (%T)", m["x"], m["x"])
		}
		nest := m["nest"].([]any)[0].(map[string]any)["id"]
		id, ok := nest.(json.Number)
		if !ok || id.String() != "9007199254740993" {
			t.Fatalf("nest id=%#v (%T)", nest, nest)
		}
	}

	v1, err := durable.Step(ctx, b, "wf", "k", fn)
	if err != nil {
		t.Fatal(err)
	}
	check(v1)
	v2, err := durable.Step(ctx, b, "wf", "k", func(context.Context) (any, error) {
		t.Fatal("replay called fn")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	check(v2)

	raw, found, err := b.Load(ctx, "wf", "k")
	if err != nil || !found {
		t.Fatalf("load found=%v err=%v", found, err)
	}
	if !json.Valid(raw) || !bytes.Contains(raw, []byte("9007199254740993")) {
		t.Fatalf("stored %s", raw)
	}
}

func TestStepLostSaveReturnsStored(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		b := durable.NewMemory()
		t.Cleanup(func() { _ = b.Close() })
		assertLostSaveReturnsStored(t, b)
	})
	t.Run("sqlite", func(t *testing.T) {
		b, err := durable.OpenLocal(filepath.Join(t.TempDir(), "memo.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close() })
		assertLostSaveReturnsStored(t, b)
	})
}

func assertLostSaveReturnsStored(t *testing.T, b durable.Backend) {
	t.Helper()
	ctx := context.Background()
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	results := make([]any, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, id := range []string{"alpha", "beta"} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			results[i], errs[i] = durable.Step(ctx, b, "wf", "race", func(context.Context) (any, error) {
				ready.Done()
				<-start
				return id, nil
			})
		}(i, id)
	}
	ready.Wait()
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if results[0] != results[1] {
		t.Fatalf("results differ: %#v vs %#v", results[0], results[1])
	}
	raw, found, err := b.Load(ctx, "wf", "race")
	if err != nil || !found {
		t.Fatalf("load found=%v err=%v", found, err)
	}
	quoted, err := json.Marshal(results[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(quoted) {
		t.Fatalf("stored %s, results %s", raw, quoted)
	}
}
