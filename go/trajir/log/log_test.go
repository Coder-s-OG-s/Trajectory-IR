package nodelog_test

import (
	"errors"
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

	ok, err := nl.Has("t1", "demo", 1, "DECISION", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected DECISION to be present")
	}

	ok, err = nl.Has("t1", "demo", 1, "TOOL_RESULT", nil)
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
	ok, err := nl.Has("t1", "demo", 1, "TOOL_CALL", &seq2)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected TOOL_CALL at seq 2")
	}

	seq4 := 4
	ok, err = nl.Has("t1", "demo", 1, "TOOL_CALL", &seq4)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("did not expect TOOL_CALL at seq 4")
	}

	// Unscoped has still finds TOOL_CALL somewhere in the step.
	ok, err = nl.Has("t1", "demo", 1, "TOOL_CALL", nil)
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

func TestClaimToolCallSingleWinner(t *testing.T) {
	nl := openTemp(t)
	claimed1, err := nl.ClaimToolCall(1, map[string]any{"tool": "a", "args": map[string]any{}}, "t1", "demo", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed1 {
		t.Fatal("first claim should win")
	}
	claimed2, err := nl.ClaimToolCall(1, map[string]any{"tool": "a", "args": map[string]any{}}, "t1", "demo", 2)
	if err != nil {
		t.Fatal(err)
	}
	if claimed2 {
		t.Fatal("second claim should lose")
	}
	seq := 2
	ok, err := nl.Has("t1", "demo", 1, "TOOL_CALL", &seq)
	if err != nil || !ok {
		t.Fatalf("TOOL_CALL present=%v err=%v", ok, err)
	}
}

func TestListNodesTenantFilter(t *testing.T) {
	nl := openTemp(t)
	step := 1
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "a"}, "t1", "tenant-a", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "b"}, "t1", "tenant-b", 2); err != nil {
		t.Fatal(err)
	}
	rows, err := nl.ListNodes("t1", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["tenant_id"] != "tenant-a" {
		t.Fatalf("rows=%v", rows)
	}
	allRows, err := nl.ListNodesAllTenants("t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(allRows) != 2 {
		t.Fatalf("allRows=%v", allRows)
	}
}

func TestPipeInTenantRejected(t *testing.T) {
	nl := openTemp(t)
	step := 1
	_, err := nl.Append("DECISION", &step, map[string]any{"plan": "x"}, "t1", "a|b", 1)
	if err == nil {
		t.Fatal("expected delimiter rejection")
	}
}

func TestAppendSlotConflictDifferentPayload(t *testing.T) {
	nl := openTemp(t)
	step := 1
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "a"}, "t1", "demo", 1); err != nil {
		t.Fatal(err)
	}
	_, err := nl.Append("DECISION", &step, map[string]any{"plan": "b"}, "t1", "demo", 1)
	if !errors.Is(err, nodelog.ErrSlotConflict) {
		t.Fatalf("err=%v want ErrSlotConflict", err)
	}
}

func TestAppendSlotConflictPreservesOriginal(t *testing.T) {
	nl := openTemp(t)
	step := 1
	n1, err := nl.Append("TOOL_RESULT", &step, map[string]any{"result": "first"}, "t1", "demo", 2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = nl.Append("TOOL_RESULT", &step, map[string]any{"result": "second"}, "t1", "demo", 2)
	if !errors.Is(err, nodelog.ErrSlotConflict) {
		t.Fatalf("err=%v want ErrSlotConflict", err)
	}

	rows, err := nl.ListNodes("t1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if rows[0]["id"] != n1.ID {
		t.Fatalf("stored id=%v want %s", rows[0]["id"], n1.ID)
	}
}

func TestHasTenantFilter(t *testing.T) {
	nl := openTemp(t)
	step := 1

	// Direct leak regression test: tenant-a writes DECISION, tenant-b must not see it.
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "a"}, "t1", "tenant-a", 1); err != nil {
		t.Fatal(err)
	}

	ok, err := nl.Has("t1", "tenant-b", 1, "DECISION", nil)
	if err != nil || ok {
		t.Fatalf("Has tenant-b=%v err=%v, want false", ok, err)
	}

	ok, err = nl.Has("t1", "tenant-a", 1, "DECISION", nil)
	if err != nil || !ok {
		t.Fatalf("Has tenant-a=%v err=%v, want true", ok, err)
	}

	// Multi-tenant slot filtering with seq
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "b"}, "t1", "tenant-b", 2); err != nil {
		t.Fatal(err)
	}

	seq2 := 2
	ok, err = nl.Has("t1", "tenant-a", 1, "DECISION", &seq2)
	if err != nil || ok {
		t.Fatalf("Has tenant-a seq2=%v err=%v", ok, err)
	}

	ok, err = nl.Has("t1", "tenant-b", 1, "DECISION", &seq2)
	if err != nil || !ok {
		t.Fatalf("Has tenant-b seq2=%v err=%v", ok, err)
	}

	ok, err = nl.Has("t1", "tenant-c", 1, "DECISION", nil)
	if err != nil || ok {
		t.Fatalf("Has tenant-c=%v err=%v", ok, err)
	}
}

func TestHasAllTenants(t *testing.T) {
	nl := openTemp(t)
	step := 1
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "a"}, "t1", "tenant-a", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "b"}, "t1", "tenant-b", 2); err != nil {
		t.Fatal(err)
	}

	// Scoped check for a third tenant is false
	ok, err := nl.Has("t1", "tenant-c", 1, "DECISION", nil)
	if err != nil || ok {
		t.Fatalf("Has tenant-c=%v err=%v, want false", ok, err)
	}

	// Cross-tenant escape hatch finds nodes from any tenant
	ok, err = nl.HasAllTenants("t1", 1, "DECISION", nil)
	if err != nil || !ok {
		t.Fatalf("HasAllTenants=%v err=%v, want true", ok, err)
	}

	seq2 := 2
	ok, err = nl.HasAllTenants("t1", 1, "DECISION", &seq2)
	if err != nil || !ok {
		t.Fatalf("HasAllTenants seq2=%v err=%v, want true", ok, err)
	}

	seq99 := 99
	ok, err = nl.HasAllTenants("t1", 1, "DECISION", &seq99)
	if err != nil || ok {
		t.Fatalf("HasAllTenants seq99=%v err=%v, want false", ok, err)
	}

	ok, err = nl.HasAllTenants("nonexistent-traj", 1, "DECISION", nil)
	if err != nil || ok {
		t.Fatalf("HasAllTenants nonexistent=%v err=%v, want false", ok, err)
	}
}

func TestHasRejectsEmptyTenant(t *testing.T) {
	nl := openTemp(t)
	_, err := nl.Has("t1", "", 1, "DECISION", nil)
	if err == nil {
		t.Fatal("expected error on empty tenantID")
	}
}


