package nodelog_test

import (
	"path/filepath"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

func openTemp(t *testing.T) *nodelog.NodeLog {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	nl, err := nodelog.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	return nl
}

func TestAppendThenHas(t *testing.T) {
	nl := openTemp(t)
	step := 1
	_, err := nl.Append(
		"DECISION",
		&step,
		map[string]any{"plan": "x"},
		"t1",
		"demo",
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := nl.Has("t1", 1, "DECISION", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected DECISION to be present")
	}

	ok, err = nl.Has("t1", 1, "TOOL_RESULT", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("did not expect TOOL_RESULT")
	}
}

func TestAppendIsIdempotentByContent(t *testing.T) {
	nl := openTemp(t)
	step := 1
	payload := map[string]any{"plan": "x"}

	n1, err := nl.Append("DECISION", &step, payload, "t1", "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := nl.Append("DECISION", &step, payload, "t1", "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if n1.ID != n2.ID {
		t.Fatalf("ids differ: %s vs %s", n1.ID, n2.ID)
	}

	count, err := nl.Count(n1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestHasWithSeqDistinguishesSlots(t *testing.T) {
	nl := openTemp(t)
	step := 1

	// Two tool call slots in one step: seq 2 and seq 4 (same pattern as Python step runner).
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{"tool": "a"}, "t1", "demo", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{"result": 1}, "t1", "demo", 3); err != nil {
		t.Fatal(err)
	}

	seq2 := 2
	ok, err := nl.Has("t1", 1, "TOOL_CALL", &seq2)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected TOOL_CALL at seq 2")
	}

	seq4 := 4
	ok, err = nl.Has("t1", 1, "TOOL_CALL", &seq4)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("did not expect TOOL_CALL at seq 4")
	}

	// Unscoped has still finds TOOL_CALL somewhere in the step.
	ok, err = nl.Has("t1", 1, "TOOL_CALL", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected unscoped TOOL_CALL hit")
	}
}

func TestStoredIdentityMatchesNodesPackage(t *testing.T) {
	nl := openTemp(t)
	step := 2
	payload := map[string]any{"a": float64(1), "b": float64(2)}

	n, err := nl.Append("STATE_SET", &step, payload, "t1", "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if n.PHash == "" || n.ID == "" {
		t.Fatal("missing hash fields on returned node")
	}

	// Same payload again: same id, still one row.
	n2, err := nl.Append("STATE_SET", &step, map[string]any{"b": float64(2), "a": float64(1)}, "t1", "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != n2.ID {
		t.Fatalf("key order changed id: %s vs %s", n.ID, n2.ID)
	}
	count, err := nl.Count(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestUnknownKindRejected(t *testing.T) {
	nl := openTemp(t)
	step := 1
	_, err := nl.Append("NOT_A_KIND", &step, map[string]any{}, "t1", "demo", 0)
	if err == nil {
		t.Fatal("expected unknown kind error")
	}
}
