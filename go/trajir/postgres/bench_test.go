package postgres

// Benchmarks for the Postgres NodeLog against a live database. They skip
// unless TRAJIR_DATABASE_URL is set. Run from go/ with the live stack from
// docs/LIVE_INTEGRATION_DOCKER.md:
//
//	go test ./trajir/postgres -run '^$' -bench . -benchmem -benchtime=1000x
//
// These measure NodeLog writes and reads only. The Go client and tir.Import
// still run on the SQLite log, so there is no Postgres step or import
// benchmark yet. Numbers are recorded in docs/PERFORMANCE.md.

import (
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"
)

var benchWords = strings.Fields(`deploy service region replica health check rollout
canary latency error budget queue worker retry config secret volume node pod
image digest manifest ticket incident owner alert metric trace span shard`)

// benchContext returns about n bytes of varied text, the same shape as the
// tir benchmarks use.
func benchContext(r *rand.Rand, n int) string {
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString(benchWords[r.IntN(len(benchWords))])
		sb.WriteByte(' ')
		if r.IntN(8) == 0 {
			fmt.Fprintf(&sb, "%x ", r.Uint64())
		}
	}
	return sb.String()
}

func openBenchLog(b *testing.B) *NodeLog {
	b.Helper()
	dsn := os.Getenv("TRAJIR_DATABASE_URL")
	if dsn == "" {
		b.Skip("set TRAJIR_DATABASE_URL to run live Postgres benchmarks")
	}
	nl, err := OpenDSN(dsn)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = nl.Close() })
	return nl
}

// benchTrajectory is unique per run so rows left by an earlier run do not
// turn appends into no-ops.
func benchTrajectory(name string) string {
	return fmt.Sprintf("bench-%s-%d", name, time.Now().UnixNano())
}

// appendStep writes the five nodes of one step: project, seal, tool call,
// tool result, commit. It is the log side of a step only, with no resume
// checks around it.
func appendStep(b *testing.B, nl *NodeLog, r *rand.Rand, traj string, step int) {
	b.Helper()
	key := fmt.Sprintf("svc-%d", r.IntN(500))
	call := map[string]any{"name": "lookup", "args": map[string]any{"key": key}}
	rows := []struct {
		kind    string
		seq     int
		payload map[string]any
	}{
		{"PROJECT_CONTEXT", 0, map[string]any{"goal": "keep the service healthy", "context": benchContext(r, 2000)}},
		{"DECISION", 1, map[string]any{"plan": map[string]any{"tool_calls": []any{call}}}},
		{"TOOL_CALL", 2, map[string]any{"tool": "lookup", "args": call["args"], "effect": "PURE"}},
		{"TOOL_RESULT", 3, map[string]any{"tool": "lookup", "result": map[string]any{"status": "ok", "key": key}}},
		{"COMMIT_STEP", 4, map[string]any{"committed": true}},
	}
	for _, row := range rows {
		if _, err := nl.Append(row.kind, &step, row.payload, traj, "bench", row.seq); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLiveAppend is one node per iteration.
func BenchmarkLiveAppend(b *testing.B) {
	nl := openBenchLog(b)
	traj := benchTrajectory("append")
	r := rand.New(rand.NewPCG(1, 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		step := i + 1
		payload := map[string]any{"goal": "keep the service healthy", "context": benchContext(r, 2000)}
		if _, err := nl.Append("PROJECT_CONTEXT", &step, payload, traj, "bench", 0); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLiveStepNodes is the five nodes of one step per iteration.
func BenchmarkLiveStepNodes(b *testing.B) {
	nl := openBenchLog(b)
	traj := benchTrajectory("step")
	r := rand.New(rand.NewPCG(1, 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		appendStep(b, nl, r, traj, i+1)
	}
}

// BenchmarkLiveListNodes reads back a whole trajectory, which is what export
// does first.
func BenchmarkLiveListNodes(b *testing.B) {
	for _, steps := range []int{100, 1000} {
		b.Run(fmt.Sprintf("steps=%d", steps), func(b *testing.B) {
			nl := openBenchLog(b)
			traj := benchTrajectory(fmt.Sprintf("list%d", steps))
			r := rand.New(rand.NewPCG(1, 2))
			for s := 1; s <= steps; s++ {
				appendStep(b, nl, r, traj, s)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rows, err := nl.ListNodes(traj, "bench")
				if err != nil {
					b.Fatal(err)
				}
				if len(rows) != steps*5 {
					b.Fatalf("rows=%d want %d", len(rows), steps*5)
				}
			}
		})
	}
}
