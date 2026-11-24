package hosthook_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

func newSession(t *testing.T) hosthook.Session {
	t.Helper()
	return hosthook.Session{
		WorkDir:      filepath.Join(t.TempDir(), "wd"),
		TenantID:     "local",
		TrajectoryID: "claude-s1",
	}
}

func readOnly(string, map[string]any) effects.EffectClass { return effects.READ_ONLY }

type row struct {
	Kind string
	Step int
	Seq  int
}

func rows(t *testing.T, s hosthook.Session) ([]row, []map[string]any) {
	t.Helper()
	nl, err := nodelog.Open(filepath.Join(s.WorkDir, "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer nl.Close()
	list, err := nl.ListNodes(s.TrajectoryID, s.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]row, 0, len(list))
	for _, n := range list {
		out = append(out, row{Kind: n["kind"].(string), Step: n["step_n"].(int), Seq: n["seq"].(int)})
	}
	return out, list
}

func mustSeal(t *testing.T, s hosthook.Session, name string, args map[string]any, id string) hosthook.Record {
	t.Helper()
	rec, err := s.SealBeforeTool(name, args, id, nil)
	if err != nil {
		t.Fatalf("seal %s: %v", id, err)
	}
	return rec
}

func TestSealWritesContextAndDecision(t *testing.T) {
	s := newSession(t)
	rec := mustSeal(t, s, "echo", map[string]any{"msg": "hi"}, "t1")
	if rec.StepN != 1 || !rec.Sealed || rec.ToolUseID != "t1" {
		t.Fatalf("record = %+v", rec)
	}
	got, list := rows(t, s)
	want := []row{{"PROJECT_CONTEXT", 1, 0}, {"DECISION", 1, 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nodes = %v, want %v", got, want)
	}
	ctx := list[0]["payload"].(map[string]any)
	if ctx["goal"] != "host-hook" || ctx["tool"] != "echo" {
		t.Fatalf("PROJECT_CONTEXT payload = %v", ctx)
	}
	plan := list[1]["payload"].(map[string]any)["plan"].(map[string]any)
	calls := plan["tool_calls"].([]any)
	call := calls[0].(map[string]any)
	if call["name"] != "echo" || call["args"].(map[string]any)["msg"] != "hi" {
		t.Fatalf("DECISION plan = %v", plan)
	}
}

func TestResultWritesCallResultCommit(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "echo", map[string]any{"msg": "hi"}, "t1")
	rec, err := s.RecordToolResult("t1", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Recorded {
		t.Fatalf("record = %+v", rec)
	}
	got, _ := rows(t, s)
	want := []row{
		{"PROJECT_CONTEXT", 1, 0}, {"DECISION", 1, 1}, {"TOOL_CALL", 1, 2},
		{"TOOL_RESULT", 1, 3}, {"COMMIT_STEP", 1, 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nodes = %v, want %v", got, want)
	}
}

func TestReadOnlyResultWritesPlainCall(t *testing.T) {
	s := newSession(t)
	s.Classify = readOnly
	mustSeal(t, s, "Read", map[string]any{"path": "a.txt"}, "t1")
	if _, err := s.RecordToolResult("t1", map[string]any{"text": "x"}); err != nil {
		t.Fatal(err)
	}
	got, _ := rows(t, s)
	if len(got) != 5 || got[3].Kind != "TOOL_RESULT" || got[4].Kind != "COMMIT_STEP" {
		t.Fatalf("nodes = %v", got)
	}
}

func TestFailureWritesAbort(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "echo", map[string]any{}, "t1")
	rec, err := s.RecordToolFailure("t1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Failed || !rec.Recorded || rec.AlreadyClaimed {
		t.Fatalf("record = %+v", rec)
	}
	got, list := rows(t, s)
	want := []row{
		{"PROJECT_CONTEXT", 1, 0}, {"DECISION", 1, 1}, {"TOOL_CALL", 1, 2},
		{"ABORT", 1, 3}, {"COMMIT_STEP", 1, 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nodes = %v, want %v", got, want)
	}
	abort := list[3]["payload"].(map[string]any)
	wantAbort := map[string]any{
		"reason": "HOST_REPORTED_FAILURE",
		"tool":   "echo",
		"error":  "host reported a tool failure",
	}
	if !reflect.DeepEqual(abort, wantAbort) {
		t.Fatalf("ABORT = %v, want %v", abort, wantAbort)
	}
}

func TestFailureErrorObjectIsJSON(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "echo", map[string]any{}, "t1")
	if _, err := s.RecordToolFailure("t1", map[string]any{"b": 1, "a": "x"}); err != nil {
		t.Fatal(err)
	}
	_, list := rows(t, s)
	if got := list[3]["payload"].(map[string]any)["error"]; got != `{"a": "x", "b": 1}` {
		t.Fatalf("error text = %q", got)
	}
}

func TestFailureErrorEscapesLikePython(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "echo", map[string]any{}, "t1")
	if _, err := s.RecordToolFailure("t1", map[string]any{"k": []any{"é", nil, true}}); err != nil {
		t.Fatal(err)
	}
	_, list := rows(t, s)
	want := "{\"k\": [\"\\" + "u00e9\", null, true]}"
	if got := list[3]["payload"].(map[string]any)["error"]; got != want {
		t.Fatalf("error text = %q", got)
	}
}

func TestFailureAfterStoredOutcomeIsAlreadyClaimed(t *testing.T) {
	s := newSession(t)
	rec := mustSeal(t, s, "echo", map[string]any{}, "t1")
	nl, err := nodelog.Open(filepath.Join(s.WorkDir, "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	step := rec.StepN
	if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{"result": "ok"}, s.TrajectoryID, s.TenantID, 3); err != nil {
		t.Fatal(err)
	}
	_ = nl.Close()
	done, err := s.RecordToolFailure("t1", "boom")
	if err != nil {
		t.Fatal(err)
	}
	if !done.Failed || done.Recorded || !done.AlreadyClaimed {
		t.Fatalf("record = %+v", done)
	}
	if got, _ := rows(t, s); len(got) != 3 {
		t.Fatalf("wrote nodes beside the stored outcome: %v", got)
	}
}

func TestCorruptStateErrors(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "echo", nil, "t1")
	matches, _ := filepath.Glob(filepath.Join(s.WorkDir, "hook_state.*.json"))
	if len(matches) != 1 {
		t.Fatalf("state files = %v", matches)
	}
	if err := os.WriteFile(matches[0], []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SealBeforeTool("echo", nil, "t2", nil); err == nil || !strings.Contains(err.Error(), matches[0]) {
		t.Fatalf("want error naming %s, got %v", matches[0], err)
	}
}

func TestNonJSONArgsRejected(t *testing.T) {
	s := newSession(t)
	if _, err := s.SealBeforeTool("echo", map[string]any{"c": make(chan int)}, "t1", nil); err == nil {
		t.Fatal("want error for non-JSON args")
	}
}

func TestFailureErrorCapped(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "echo", map[string]any{}, "t1")
	if _, err := s.RecordToolFailure("t1", strings.Repeat("x", 5000)); err != nil {
		t.Fatal(err)
	}
	_, list := rows(t, s)
	if got := list[3]["payload"].(map[string]any)["error"].(string); len(got) != 2000 {
		t.Fatalf("error len = %d, want 2000", len(got))
	}
}

func TestSameOwnerRepeatPendingGatedIsReplay(t *testing.T) {
	s := newSession(t)
	s.Owner = "project"
	mustSeal(t, s, "bash", map[string]any{"command": "deploy"}, "t1")
	rec := mustSeal(t, s, "bash", map[string]any{"command": "deploy"}, "t1")
	if !rec.Replay || rec.Duplicate || !hosthook.ReplayBlocked(rec) {
		t.Fatalf("record = %+v", rec)
	}
	if got, _ := rows(t, s); len(got) != 2 {
		t.Fatalf("repeat wrote nodes: %v", got)
	}
}

func TestSameOwnerRepeatPendingReadOnlyIsDuplicate(t *testing.T) {
	s := newSession(t)
	s.Classify = readOnly
	mustSeal(t, s, "Read", map[string]any{}, "t1")
	rec := mustSeal(t, s, "Read", map[string]any{}, "t1")
	if rec.Replay || !rec.Duplicate || hosthook.ReplayBlocked(rec) {
		t.Fatalf("record = %+v", rec)
	}
}

func TestOtherOwnerIsDuplicate(t *testing.T) {
	s := newSession(t)
	s.Owner = "a"
	mustSeal(t, s, "bash", map[string]any{}, "t1")
	s.Owner = "b"
	rec := mustSeal(t, s, "bash", map[string]any{}, "t1")
	if rec.Replay || !rec.Duplicate || hosthook.ReplayBlocked(rec) {
		t.Fatalf("record = %+v", rec)
	}
}

func TestDoneIdIsReplay(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "bash", map[string]any{}, "t1")
	if _, err := s.RecordToolResult("t1", "ok"); err != nil {
		t.Fatal(err)
	}
	rec := mustSeal(t, s, "bash", map[string]any{}, "t1")
	if !rec.Replay || !rec.Recorded || !hosthook.ReplayBlocked(rec) {
		t.Fatalf("record = %+v", rec)
	}
	if got, _ := rows(t, s); len(got) != 5 {
		t.Fatalf("replay wrote nodes: %v", got)
	}
}

func TestResultTwiceReturnsDone(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "bash", map[string]any{}, "t1")
	if _, err := s.RecordToolResult("t1", "ok"); err != nil {
		t.Fatal(err)
	}
	rec, err := s.RecordToolResult("t1", "ok")
	if err != nil || !rec.Recorded {
		t.Fatalf("second result: %+v, %v", rec, err)
	}
	if got, _ := rows(t, s); len(got) != 5 {
		t.Fatalf("second result wrote nodes: %v", got)
	}
}

func TestResultWithoutSealIsErrNoSeal(t *testing.T) {
	s := newSession(t)
	if _, err := s.RecordToolResult("nope", "x"); !errors.Is(err, hosthook.ErrNoSeal) {
		t.Fatalf("result err = %v", err)
	}
	if _, err := s.RecordToolFailure("nope", "x"); !errors.Is(err, hosthook.ErrNoSeal) {
		t.Fatalf("failure err = %v", err)
	}
}

func TestEmptyToolUseIDRejected(t *testing.T) {
	s := newSession(t)
	if _, err := s.SealBeforeTool("echo", nil, "", nil); err == nil {
		t.Fatal("want error for empty tool_use_id")
	}
}

func TestSecondStepIncrements(t *testing.T) {
	s := newSession(t)
	a := mustSeal(t, s, "echo", nil, "t1")
	b := mustSeal(t, s, "echo", nil, "t2")
	if a.StepN != 1 || b.StepN != 2 {
		t.Fatalf("steps = %d, %d", a.StepN, b.StepN)
	}
}

func TestTrajectoriesDoNotShareSteps(t *testing.T) {
	s := newSession(t)
	other := s
	other.TrajectoryID = "claude-s2"
	mustSeal(t, s, "echo", nil, "t1")
	if rec := mustSeal(t, other, "echo", nil, "t1"); rec.StepN != 1 || rec.Replay {
		t.Fatalf("other trajectory record = %+v", rec)
	}
}

func TestParallelSealsGetDistinctSteps(t *testing.T) {
	s := newSession(t)
	var wg sync.WaitGroup
	steps := make([]int, 8)
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec, err := s.SealBeforeTool("echo", map[string]any{"i": i}, "t"+string(rune('a'+i)), nil)
			steps[i], errs[i] = rec.StepN, err
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Ints(steps)
	if !reflect.DeepEqual(steps, []int{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("steps = %v", steps)
	}
}

func TestTrajectoryIDDoesNotEscapeWorkDir(t *testing.T) {
	parent := t.TempDir()
	s := hosthook.Session{
		WorkDir:      filepath.Join(parent, "wd"),
		TenantID:     "local",
		TrajectoryID: `../../x:y\z`,
	}
	mustSeal(t, s, "echo", nil, "t1")
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "wd" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("files outside work dir: %v", names)
	}
}
