package postgres

import (
	"errors"
	"os"
	"testing"
)

func openTestLog(t *testing.T) *NodeLog {
	t.Helper()
	dsn := os.Getenv("TRAJIR_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TRAJIR_DATABASE_URL to run live Postgres NodeLog tests")
	}
	nl, err := OpenDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	return nl
}

func TestOpenFromEnvRequiresDSN(t *testing.T) {
	t.Setenv("TRAJIR_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")
	_, err := OpenFromEnv()
	if err == nil {
		t.Fatal("expected error without DSN")
	}
}

func TestOpenDSNEmpty(t *testing.T) {
	_, err := OpenDSN("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLiveAppendHasListIdempotent(t *testing.T) {
	nl := openTestLog(t)
	traj := "go-live-" + t.Name()
	step := 1
	n1, err := nl.Append("DECISION", &step, map[string]any{"plan": "x"}, traj, "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := nl.Append("DECISION", &step, map[string]any{"plan": "x"}, traj, "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if n1.ID != n2.ID {
		t.Fatalf("ids differ %s vs %s", n1.ID, n2.ID)
	}
	ok, err := nl.Has(traj, "demo", 1, "DECISION", nil)
	if err != nil || !ok {
		t.Fatalf("Has=%v err=%v", ok, err)
	}
	rows, err := nl.ListNodes(traj, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
}

func TestLiveSlotConflict(t *testing.T) {
	nl := openTestLog(t)
	traj := "go-conflict-" + t.Name()
	step := 1
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": "a"}, traj, "demo", 1); err != nil {
		t.Fatal(err)
	}
	_, err := nl.Append("DECISION", &step, map[string]any{"plan": "b"}, traj, "demo", 1)
	if !errors.Is(err, ErrSlotConflict) {
		t.Fatalf("err=%v want ErrSlotConflict", err)
	}
}

func TestLiveClaimToolCall(t *testing.T) {
	nl := openTestLog(t)
	traj := "go-claim-" + t.Name()
	claimed, err := nl.ClaimToolCall(1, map[string]any{"tool": "x"}, traj, "demo", 2)
	if err != nil || !claimed {
		t.Fatalf("first claim claimed=%v err=%v", claimed, err)
	}
	claimed2, err := nl.ClaimToolCall(1, map[string]any{"tool": "x"}, traj, "demo", 2)
	if err != nil {
		t.Fatal(err)
	}
	if claimed2 {
		t.Fatal("second claim should lose")
	}
}
