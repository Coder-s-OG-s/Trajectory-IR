package hosthook_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
)

func TestSealWithoutIDGetsDerivedID(t *testing.T) {
	s := newSession(t)
	rec := mustSeal(t, s, "read_file", map[string]any{"path": "a"}, "")
	if !strings.HasPrefix(rec.ToolUseID, "d-") || rec.StepN != 1 {
		t.Fatalf("record = %+v", rec)
	}
}

func TestDerivedIDPairsFIFO(t *testing.T) {
	s := newSession(t)
	args := map[string]any{"path": "a"}
	first := mustSeal(t, s, "read_file", args, "")
	second := mustSeal(t, s, "read_file", args, "")
	mustSeal(t, s, "other", map[string]any{}, "")
	if first.ToolUseID == second.ToolUseID {
		t.Fatalf("same derived id twice: %s", first.ToolUseID)
	}
	got, err := s.RecordMatchedResult("read_file", map[string]any{"path": "a"}, "x")
	if err != nil {
		t.Fatal(err)
	}
	if got.ToolUseID != first.ToolUseID || !got.Recorded {
		t.Fatalf("matched %+v, want %s", got, first.ToolUseID)
	}
	got, err = s.RecordMatchedFailure("read_file", args, "gone")
	if err != nil {
		t.Fatal(err)
	}
	if got.ToolUseID != second.ToolUseID || !got.Failed {
		t.Fatalf("matched %+v, want %s", got, second.ToolUseID)
	}
}

func TestMatchUsesArgsToo(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "read_file", map[string]any{"path": "a"}, "t1")
	if _, err := s.RecordMatchedResult("read_file", map[string]any{"path": "b"}, "x"); !errors.Is(err, hosthook.ErrNoSeal) {
		t.Fatalf("err = %v", err)
	}
	rec, err := s.RecordMatchedResult("read_file", map[string]any{"path": "a"}, "x")
	if err != nil || rec.ToolUseID != "t1" {
		t.Fatalf("host-id call not matched: %+v, %v", rec, err)
	}
}

func TestMatchNumbersCompareByValue(t *testing.T) {
	s := newSession(t)
	mustSeal(t, s, "f", map[string]any{"n": 3}, "")
	if _, err := s.RecordMatchedResult("f", map[string]any{"n": float64(3)}, "x"); err != nil {
		t.Fatalf("3 and 3.0 did not match: %v", err)
	}
}

func owners(t *testing.T) (hosthook.Session, hosthook.Session) {
	t.Helper()
	project := newSession(t)
	project.Owner = "project"
	user := project
	user.Owner = "user"
	return project, user
}

// A project install and a user install both hook a host that sends no id.
// The second pre hook joins the first one's call, so one call is one step,
// whichever post hook runs first.
func TestSecondOwnerJoinsDerivedCall(t *testing.T) {
	for _, userPostsFirst := range []bool{false, true} {
		project, user := owners(t)
		args := map[string]any{"command": "ls"}
		first := mustSeal(t, project, "run_shell_command", args, "")
		second := mustSeal(t, user, "run_shell_command", args, "")
		if !second.Duplicate || second.ToolUseID != first.ToolUseID || second.StepN != first.StepN {
			t.Fatalf("second owner = %+v, want a duplicate of %+v", second, first)
		}
		posts := []hosthook.Session{project, user}
		if userPostsFirst {
			posts = []hosthook.Session{user, project}
		}
		for _, s := range posts {
			if _, err := s.RecordMatchedResult("run_shell_command", args, "out"); err != nil {
				t.Fatalf("%s post: %v", s.Owner, err)
			}
		}
		got, _ := rows(t, project)
		want := []row{{"PROJECT_CONTEXT", 1, 0}, {"DECISION", 1, 1}, {"TOOL_CALL", 1, 2}, {"TOOL_RESULT", 1, 3}, {"COMMIT_STEP", 1, 4}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("user posts first=%v: rows = %v", userPostsFirst, got)
		}
	}
}

func TestSecondOwnerJoinsDerivedFailure(t *testing.T) {
	project, user := owners(t)
	args := map[string]any{"absolute_path": "a"}
	mustSeal(t, project, "read_file", args, "")
	mustSeal(t, user, "read_file", args, "")
	for _, s := range []hosthook.Session{project, user} {
		if _, err := s.RecordMatchedFailure("read_file", args, "File not found"); err != nil {
			t.Fatalf("%s post: %v", s.Owner, err)
		}
	}
	got, _ := rows(t, project)
	want := []row{{"PROJECT_CONTEXT", 1, 0}, {"DECISION", 1, 1}, {"TOOL_CALL", 1, 2}, {"ABORT", 1, 3}, {"COMMIT_STEP", 1, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v", got)
	}
}

// The next identical call is still a new step for both owners.
func TestTwoOwnersRepeatCallIsNewStep(t *testing.T) {
	project, user := owners(t)
	args := map[string]any{"command": "ls"}
	for i := 1; i <= 2; i++ {
		rec := mustSeal(t, project, "run_shell_command", args, "")
		mustSeal(t, user, "run_shell_command", args, "")
		for _, s := range []hosthook.Session{project, user} {
			if _, err := s.RecordMatchedResult("run_shell_command", args, "out"); err != nil {
				t.Fatal(err)
			}
		}
		if rec.StepN != i {
			t.Fatalf("call %d sealed as step %d", i, rec.StepN)
		}
	}
	got, _ := rows(t, project)
	if len(got) != 10 {
		t.Fatalf("want 2 steps of 5 nodes, rows = %v", got)
	}
}
