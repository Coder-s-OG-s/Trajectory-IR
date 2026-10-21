package nodelog_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"

	_ "modernc.org/sqlite"
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

func TestClaimToolCallNonConstraintErrorPropagates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	nl, err := nodelog.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	side, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = side.Close() })

	// Add an abort trigger on insert that does NOT represent a unique constraint
	if _, err := side.Exec(`CREATE TRIGGER abort_insert BEFORE INSERT ON nodes BEGIN SELECT RAISE(ABORT, 'forced abort error'); END;`); err != nil {
		t.Fatal(err)
	}

	claimed, err := nl.ClaimToolCall(1, map[string]any{"tool": "a"}, "t1", "demo", 2)
	if err == nil {
		t.Fatal("expected non-constraint error to be propagated, got nil")
	}
	if !strings.Contains(err.Error(), "forced abort error") {
		t.Fatalf("expected error to contain 'forced abort error', got %v", err)
	}
	if claimed {
		t.Fatal("expected claimed=false when insert fails")
	}
}

func TestClaimToolCallUniqueConstraintRaceLostReturnsFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	nl, err := nodelog.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	side, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = side.Close() })

	// Simulate a race loser where another transaction slips in an identical slot
	// right before our INSERT executes, causing an idx_nodes_slot unique violation on INSERT.
	_, err = side.Exec(`
		CREATE TRIGGER race_insert BEFORE INSERT ON nodes
		WHEN NEW.id != 'winner'
		BEGIN
			INSERT INTO nodes (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts)
			VALUES ('winner', NEW.trajectory_id, NEW.tenant_id, NEW.step_n, NEW.seq, 'TOOL_CALL', '{}', NEW.ts);
		END;
	`)
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := nl.ClaimToolCall(1, map[string]any{"tool": "a"}, "t1", "demo", 2)
	if err != nil {
		t.Fatalf("expected nil error on unique constraint collision, got: %v", err)
	}
	if claimed {
		t.Fatal("expected claimed=false when losing race to unique constraint")
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

func TestAppendErrorsWhenSlotOwnerMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	nl, err := nodelog.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	step := 1
	n, err := nl.Append("THOUGHT", &step, map[string]any{"text": "hi"}, "traj", "tenant", 0)
	if err != nil {
		t.Fatal(err)
	}

	side, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = side.Close() })
	if _, err := side.Exec(`UPDATE nodes SET step_n = 99 WHERE id = ?`, n.ID); err != nil {
		t.Fatal(err)
	}

	_, err = nl.Append("THOUGHT", &step, map[string]any{"text": "hi"}, "traj", "tenant", 0)
	if !errors.Is(err, nodelog.ErrNotStored) {
		t.Fatalf("err=%v want ErrNotStored", err)
	}

	var atStep int
	if err := side.QueryRow(`SELECT COUNT(*) FROM nodes WHERE IFNULL(step_n, -1) = 1`).Scan(&atStep); err != nil {
		t.Fatal(err)
	}
	if atStep != 0 {
		t.Fatalf("step 1 rows=%d", atStep)
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

// Rows written by an older version or another writer may hold ids the write
// path would now refuse. Reading must still work.
func TestReadPathsStillServeRowsWithLegacyIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	nl, err := nodelog.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	side, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = side.Close() })

	longID := strings.Repeat("t", 600)
	for i, tc := range []struct{ traj, tenant string }{{"legacy|traj", "legacy|tenant"}, {longID, "demo"}} {
		if _, err := side.Exec(
			`INSERT INTO nodes (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts) VALUES (?, ?, ?, 1, ?, 'THOUGHT', '{"n":1}', 1.0)`,
			fmt.Sprintf("legacy-%d", i), tc.traj, tc.tenant, i,
		); err != nil {
			t.Fatal(err)
		}
		got, err := nl.ListNodes(tc.traj, tc.tenant)
		if err != nil || len(got) != 1 {
			t.Fatalf("ListNodes(%q,%q): rows=%d err=%v", tc.traj, tc.tenant, len(got), err)
		}
		if all, err := nl.ListNodesAllTenants(tc.traj); err != nil || len(all) != 1 {
			t.Fatalf("ListNodesAllTenants(%q): rows=%d err=%v", tc.traj, len(all), err)
		}
		ok, err := nl.Has(tc.traj, tc.tenant, 1, "THOUGHT", nil)
		if err != nil || !ok {
			t.Fatalf("Has(%q,%q) = %v, %v", tc.traj, tc.tenant, ok, err)
		}
		if ok, err := nl.HasAllTenants(tc.traj, 1, "THOUGHT", nil); err != nil || !ok {
			t.Fatalf("HasAllTenants(%q) = %v, %v", tc.traj, ok, err)
		}
	}
}

func TestListNodesKeepsLargeIntegersAndRejectsCorruptPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodes.sqlite")
	nl, err := nodelog.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	side, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = side.Close() })

	insert := func(id string, seq int, payload string) {
		t.Helper()
		if _, err := side.Exec(
			`INSERT INTO nodes (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts) VALUES (?, 't1', 'demo', 1, ?, 'THOUGHT', ?, 1.0)`,
			id, seq, payload,
		); err != nil {
			t.Fatal(err)
		}
	}
	insert("a", 0, `{"big":9007199254740993}`)
	rows, err := nl.ListNodes("t1", "demo")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	payload := rows[0]["payload"].(map[string]any)
	if n, ok := payload["big"].(json.Number); !ok || n.String() != "9007199254740993" {
		t.Fatalf("big=%#v, want exact json.Number", payload["big"])
	}

	insert("b", 1, `{"x":1} trailing`)
	if _, err := nl.ListNodes("t1", "demo"); err == nil {
		t.Fatal("expected a payload_json with trailing data to be rejected")
	}
}
