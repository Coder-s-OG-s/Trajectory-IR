package resume_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

func TestDecisionPayloadOmitsEmptySnapshot(t *testing.T) {
	p := resume.DecisionPayload(map[string]any{"tool_calls": []any{}}, nil)
	if _, ok := p["world_snapshot"]; ok {
		t.Fatalf("empty snapshot must be omitted: %#v", p)
	}
	p = resume.DecisionPayload(map[string]any{"tool_calls": []any{}}, map[string]string{})
	if _, ok := p["world_snapshot"]; ok {
		t.Fatalf("empty map must be omitted: %#v", p)
	}
}

func TestCheckWorldNoopWithoutSnapshot(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	step := 1
	if _, err := nl.Append("DECISION", &step, resume.DecisionPayload(map[string]any{}, nil), "t1", "demo", 1); err != nil {
		t.Fatal(err)
	}
	if err := resume.CheckWorld(nl, "t1", "demo", 1, map[string]string{"cluster_generation": "99"}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckWorldPassAndDrift(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	step := 1
	payload := resume.DecisionPayload(map[string]any{}, map[string]string{
		"cluster_generation": "42",
		"node":               "i-123",
	})
	if _, err := nl.Append("DECISION", &step, payload, "t1", "demo", 1); err != nil {
		t.Fatal(err)
	}
	if err := resume.CheckWorld(nl, "t1", "demo", 1, map[string]string{
		"cluster_generation": "42",
		"node":               "i-123",
		"extra":              "ignored",
	}); err != nil {
		t.Fatal(err)
	}
	err = resume.CheckWorld(nl, "t1", "demo", 1, map[string]string{"cluster_generation": "43", "node": "i-123"})
	var drift *resume.WorldDrift
	if !errors.As(err, &drift) {
		t.Fatalf("err=%v want WorldDrift", err)
	}
	if drift.StepN != 1 || len(drift.Mismatches) != 1 {
		t.Fatalf("drift=%#v", drift)
	}
}

func TestCheckWorldRequiresDecision(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	err = resume.CheckWorld(nl, "t1", "demo", 1, map[string]string{"k": "v"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func echoPlan(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{
		"tool_calls": []any{
			map[string]any{"name": "echo", "args": map[string]any{"msg": "ok"}},
		},
	}, nil
}

func TestRunStepObserveWorldDrift(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	backend := durable.NewMemory()
	t.Cleanup(func() { _ = backend.Close() })

	observed := map[string]string{"cluster_generation": "1"}
	cfg := resume.RunStepConfig{
		Log:           nl,
		Backend:       backend,
		TenantID:      "demo",
		TrajectoryID:  "t-world",
		WorkflowID:    "wf-world",
		WorldSnapshot: map[string]string{"cluster_generation": "1"},
		ObserveWorld: func() (map[string]string, error) {
			out := map[string]string{}
			for k, v := range observed {
				out[k] = v
			}
			return out, nil
		},
		Tools: map[string]resume.Tool{
			"echo": {
				Name:   "echo",
				Effect: effects.PURE,
				Fn:     func(args map[string]any) (any, error) { return args["msg"], nil },
			},
		},
	}
	results, err := resume.RunStep(context.Background(), cfg, 1, echoPlan, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0] != "ok" {
		t.Fatalf("results=%#v", results)
	}

	observed["cluster_generation"] = "2"
	_, err = resume.RunStep(context.Background(), cfg, 2, echoPlan, map[string]any{})
	var drift *resume.WorldDrift
	if !errors.As(err, &drift) {
		t.Fatalf("err=%v want WorldDrift", err)
	}
}

func TestRunStepResumeAfterDecisionReportsDriftNotSlotConflict(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	backend := durable.NewMemory()
	t.Cleanup(func() { _ = backend.Close() })

	gen := "1"
	observes := 0
	cfg := resume.RunStepConfig{
		Log:           nl,
		Backend:       backend,
		TenantID:      "demo",
		TrajectoryID:  "t-resume-world",
		WorkflowID:    "wf-resume-world",
		WorldSnapshot: map[string]string{"cluster_generation": "1"},
		ObserveWorld: func() (map[string]string, error) {
			observes++
			if observes == 1 {
				return nil, errors.New("crash after decision")
			}
			return map[string]string{"cluster_generation": gen}, nil
		},
		Tools: map[string]resume.Tool{
			"echo": {
				Name:   "echo",
				Effect: effects.PURE,
				Fn:     func(args map[string]any) (any, error) { return args["msg"], nil },
			},
		},
	}
	_, err = resume.RunStep(context.Background(), cfg, 1, echoPlan, map[string]any{})
	if err == nil || err.Error() != "crash after decision" {
		t.Fatalf("first run err=%v", err)
	}

	cfg.WorldSnapshot = map[string]string{"cluster_generation": "2"}
	gen = "2"
	_, err = resume.RunStep(context.Background(), cfg, 1, echoPlan, map[string]any{})
	var drift *resume.WorldDrift
	if !errors.As(err, &drift) {
		t.Fatalf("err=%v want WorldDrift, not a slot conflict", err)
	}
	if drift.StepN != 1 || len(drift.Mismatches) != 1 || drift.Mismatches[0].Sealed != "1" {
		t.Fatalf("drift=%#v", drift)
	}
}
