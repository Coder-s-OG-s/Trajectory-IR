package hosthook_test

import (
	"errors"
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
