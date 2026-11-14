package client_test

// Cost of one full agent step on the default SQLite log and LocalSQLite
// backend. Run from go/:
//
//	go test ./trajir/client -run '^$' -bench . -benchmem -benchtime=1000x
//
// Use a fixed -benchtime. Each step syncs SQLite to disk, and the default
// auto ramp makes the result swing a lot from run to run.
//
// Numbers and hardware are recorded in docs/PERFORMANCE.md.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

// BenchmarkStep is project + seal + one PURE tool + commit, about 2 KB of
// context per step, all appended to one growing trajectory.
func BenchmarkStep(b *testing.B) {
	tr, err := client.OpenTrajectory("bench", "bench-step", client.Options{WorkDir: b.TempDir()})
	if err != nil {
		b.Fatal(err)
	}
	defer tr.Close()
	tool := resume.Tool{
		Name:   "lookup",
		Effect: effects.PURE,
		Fn: func(args map[string]any) (any, error) {
			return map[string]any{"status": "ok", "key": args["key"]}, nil
		},
	}
	ctx := strings.Repeat("context ", 250)
	step := 0
	for b.Loop() {
		step++
		key := fmt.Sprintf("svc-%d", step)
		if _, err := tr.Project(step, map[string]any{"goal": "bench", "context": ctx, "n": step}); err != nil {
			b.Fatal(err)
		}
		plan := map[string]any{"tool_calls": []any{map[string]any{"name": "lookup", "args": map[string]any{"key": key}}}}
		if _, err := tr.SealDecision(step, plan); err != nil {
			b.Fatal(err)
		}
		if _, err := tr.ExecTool(step, 2, tool, map[string]any{"key": key}); err != nil {
			b.Fatal(err)
		}
		if err := tr.CommitStep(step, 4); err != nil {
			b.Fatal(err)
		}
	}
}
