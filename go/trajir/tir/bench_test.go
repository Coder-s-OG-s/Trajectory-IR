package tir_test

// Benchmarks for the .tir path: export, import and verify at a few trajectory
// sizes. Run from go/:
//
//	go test ./trajir/tir -run '^$' -bench . -benchmem
//
// Numbers and hardware are recorded in docs/PERFORMANCE.md.

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/audit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

var benchSizes = []int{100, 1000}

var benchWords = strings.Fields(`deploy service region replica health check rollout
canary latency error budget queue worker retry config secret volume node pod
image digest manifest ticket incident owner alert metric trace span shard`)

// benchContext returns about n bytes of varied text so packages do not
// compress unrealistically well.
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

// buildTrajectory writes steps full agent steps (project, seal, one PURE
// tool, commit) and returns the open trajectory. The caller closes it.
func buildTrajectory(b *testing.B, dir string, steps int) *client.Trajectory {
	b.Helper()
	tr, err := client.OpenTrajectory("bench", "bench", client.Options{WorkDir: dir})
	if err != nil {
		b.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2))
	tool := resume.Tool{
		Name:   "lookup",
		Effect: effects.PURE,
		Fn: func(args map[string]any) (any, error) {
			return map[string]any{"status": "ok", "key": args["key"]}, nil
		},
	}
	for i := 1; i <= steps; i++ {
		key := fmt.Sprintf("svc-%d", r.IntN(500))
		if _, err := tr.Project(i, map[string]any{"goal": "keep the service healthy", "context": benchContext(r, 2000)}); err != nil {
			b.Fatal(err)
		}
		plan := map[string]any{"tool_calls": []any{map[string]any{"name": "lookup", "args": map[string]any{"key": key}}}}
		if _, err := tr.SealDecision(i, plan); err != nil {
			b.Fatal(err)
		}
		if _, err := tr.ExecTool(i, 2, tool, map[string]any{"key": key}); err != nil {
			b.Fatal(err)
		}
		if err := tr.CommitStep(i, 4); err != nil {
			b.Fatal(err)
		}
	}
	return tr
}

func exportPackage(b *testing.B, steps int) string {
	b.Helper()
	dir := b.TempDir()
	tr := buildTrajectory(b, dir, steps)
	defer tr.Close()
	out := filepath.Join(dir, "bench.tir")
	if _, err := tir.Export(tr.Log(), "bench", out, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		b.Fatal(err)
	}
	return out
}

func BenchmarkExport(b *testing.B) {
	for _, steps := range benchSizes {
		b.Run(fmt.Sprintf("steps=%d", steps), func(b *testing.B) {
			dir := b.TempDir()
			tr := buildTrajectory(b, dir, steps)
			defer tr.Close()
			b.ResetTimer()
			var out string
			for i := 0; b.Loop(); i++ {
				out = filepath.Join(dir, fmt.Sprintf("out-%d.tir", i))
				if _, err := tir.Export(tr.Log(), "bench", out, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if fi, err := os.Stat(out); err == nil {
				b.ReportMetric(float64(fi.Size())/1024, "KB/pkg")
			}
		})
	}
}

func BenchmarkImport(b *testing.B) {
	for _, steps := range benchSizes {
		b.Run(fmt.Sprintf("steps=%d", steps), func(b *testing.B) {
			pkg := exportPackage(b, steps)
			dir := b.TempDir()
			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				b.StopTimer()
				dest, err := nodelog.Open(filepath.Join(dir, fmt.Sprintf("dest-%d.sqlite", i)))
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if _, err := tir.Import(pkg, dest); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				dest.Close()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkVerify measures what `trajir verify` runs: hash, seal before
// execute, open world and signature checks.
func BenchmarkVerify(b *testing.B) {
	for _, steps := range benchSizes {
		b.Run(fmt.Sprintf("steps=%d", steps), func(b *testing.B) {
			pkg := exportPackage(b, steps)
			b.ResetTimer()
			for b.Loop() {
				res, err := audit.VerifyFile(pkg, audit.Options{})
				if err != nil {
					b.Fatal(err)
				}
				if !res.OK {
					b.Fatalf("verify failed: %v", res.Findings)
				}
			}
		})
	}
}
