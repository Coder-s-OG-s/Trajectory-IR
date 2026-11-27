package hosthook_test

import (
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/console"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

// The hook writes through the same file sink the console reads, and the
// dashboard shows the run with its seals and tool calls.
func TestHookStepsShowInDashboard(t *testing.T) {
	data := filepath.Join(t.TempDir(), "console-data")
	s := newSession(t)
	s.Sink = &emit.FileSink{Dir: data}
	s.Runtime = "claude-code"

	mustSeal(t, s, "deploy", map[string]any{"env": "prod"}, "t1")
	if _, err := s.RecordToolResult("t1", "ok"); err != nil {
		t.Fatal(err)
	}
	mustSeal(t, s, "Read", map[string]any{"path": "a"}, "t2")
	if _, err := s.RecordToolFailure("t2", "missing"); err != nil {
		t.Fatal(err)
	}

	store, err := console.OpenStore(data)
	if err != nil {
		t.Fatal(err)
	}
	dash, err := store.Dashboard()
	if err != nil {
		t.Fatal(err)
	}
	if len(dash.Runs) != 1 || dash.Runs[0].TrajectoryID != s.TrajectoryID {
		t.Fatalf("dashboard runs = %+v", dash.Runs)
	}
	run := dash.Runs[0]
	if run.SealCreatedCount != 2 || run.NodeCount != 10 {
		t.Fatalf("run card = %+v", run)
	}

	events, err := store.ReadEvents(s.TrajectoryID)
	if err != nil {
		t.Fatal(err)
	}
	ev := console.Summarize(s.TrajectoryID, events).Evidence
	if ev.SealBeforeExecuteOK == nil || !*ev.SealBeforeExecuteOK {
		t.Fatalf("seal before execute = %v, gaps %+v", ev.SealBeforeExecuteOK, ev.SealBeforeExecuteGaps)
	}
	if len(ev.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v", ev.ToolCalls)
	}
	first := ev.ToolCalls[0]
	if first.ToolName != "deploy" || first.EffectClass != "NON_IDEMPOTENT_WRITE" || first.IdempotencyKey == "" {
		t.Fatalf("first tool call = %+v", first)
	}
}
