package postgres

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestResolvePGConnectTimeout(t *testing.T) {
	t.Setenv("TRAJIR_PG_CONNECT_TIMEOUT", "")
	dsn := "postgres://trajir:trajir@127.0.0.1:5432/trajir?sslmode=disable"
	got, err := resolvePGConnectTimeout(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if got != 10*time.Second {
		t.Fatalf("timeout=%s", got)
	}
	effective, err := dsnWithConnectTimeout(dsn, got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(effective, "connect_timeout=10") {
		t.Fatalf("dsn=%s", effective)
	}

	withTimeout := dsn + "&connect_timeout=4"
	got, err = resolvePGConnectTimeout(withTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4*time.Second {
		t.Fatalf("dsn timeout=%s", got)
	}
	same, err := dsnWithConnectTimeout(withTimeout, got)
	if err != nil {
		t.Fatal(err)
	}
	if same != withTimeout {
		t.Fatalf("rewrote unchanged dsn: %s", same)
	}

	t.Setenv("TRAJIR_PG_CONNECT_TIMEOUT", "6")
	got, err = resolvePGConnectTimeout(withTimeout)
	if err != nil {
		t.Fatal(err)
	}
	if got != 6*time.Second {
		t.Fatalf("env timeout=%s", got)
	}
	overridden, err := dsnWithConnectTimeout(withTimeout, got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(overridden, "connect_timeout=6") || strings.Contains(overridden, "connect_timeout=4") {
		t.Fatalf("overridden=%s", overridden)
	}

	keyword := "host=127.0.0.1 user=trajir password=trajir dbname=trajir sslmode=disable"
	t.Setenv("TRAJIR_PG_CONNECT_TIMEOUT", "")
	got, err = resolvePGConnectTimeout(keyword)
	if err != nil {
		t.Fatal(err)
	}
	if got != 10*time.Second {
		t.Fatalf("keyword timeout=%s", got)
	}
	kw, err := dsnWithConnectTimeout(keyword, got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(kw, "connect_timeout=10") {
		t.Fatalf("keyword dsn=%s", kw)
	}

	t.Setenv("TRAJIR_PG_CONNECT_TIMEOUT", "nope")
	if _, err := resolvePGConnectTimeout(dsn); err == nil {
		t.Fatal("expected invalid env error")
	}
}

func TestOpenDBNil(t *testing.T) {
	_, err := OpenDB(nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func expectDDL(mock sqlmock.Sqlmock) {
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS nodes").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_slot").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_nodes_traj_tenant").WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestMockAppendHasListClose(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectDDL(mock)

	nl, err := OpenDB(db)
	if err != nil {
		t.Fatal(err)
	}

	step := 1
	mock.ExpectBegin()
	mock.ExpectExec("SAVEPOINT insert_node").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO nodes").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT id FROM nodes").WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()

	n, err := nl.Append("DECISION", &step, map[string]any{"plan": "x"}, "t1", "demo", 1)
	if err != nil {
		t.Fatal(err)
	}
	if n.ID == "" {
		t.Fatal("empty id")
	}

	mock.ExpectQuery("SELECT 1 FROM nodes WHERE trajectory_id").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	ok, err := nl.Has("t1", "demo", 1, "DECISION", nil)
	if err != nil || !ok {
		t.Fatalf("Has=%v err=%v", ok, err)
	}

	seq := 1
	mock.ExpectQuery("SELECT 1 FROM nodes WHERE trajectory_id").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	ok, err = nl.Has("t1", "demo", 1, "DECISION", &seq)
	if err != nil || !ok {
		t.Fatalf("Has seq=%v err=%v", ok, err)
	}

	mock.ExpectQuery("SELECT 1 FROM nodes WHERE trajectory_id").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	ok, err = nl.HasAllTenants("t1", 1, "DECISION", nil)
	if err != nil || !ok {
		t.Fatalf("HasAllTenants=%v err=%v", ok, err)
	}

	if _, err := nl.Has("t1", "", 1, "DECISION", nil); err == nil {
		t.Fatal("expected error on empty tenantID")
	}

	mock.ExpectQuery("SELECT id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "trajectory_id", "tenant_id", "step_n", "seq", "kind", "payload_json", "ts",
		}).AddRow(n.ID, "t1", "demo", 1, 1, "DECISION", `{"plan":"x"}`, 1.5))
	rows, err := nl.ListNodes("t1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["kind"] != "DECISION" {
		t.Fatalf("rows=%v", rows)
	}

	mock.ExpectQuery("SELECT id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "trajectory_id", "tenant_id", "step_n", "seq", "kind", "payload_json", "ts",
		}).AddRow(n.ID, "t1", "demo", nil, 1, "DECISION", `{"plan":"x"}`, 1.5))
	rows, err = nl.ListNodesAllTenants("t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("all tenants rows=%d", len(rows))
	}

	mock.ExpectClose()
	if err := nl.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMockClaimToolCallWinAndLose(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectDDL(mock)

	nl, err := OpenDB(db)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT 1 FROM nodes").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO nodes").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	claimed, err := nl.ClaimToolCall(1, map[string]any{"tool": "x"}, "t1", "demo", 2)
	if err != nil || !claimed {
		t.Fatalf("claimed=%v err=%v", claimed, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT 1 FROM nodes").
		WillReturnRows(sqlmock.NewRows([]string{"?column?"}).AddRow(1))
	mock.ExpectCommit()
	claimed, err = nl.ClaimToolCall(1, map[string]any{"tool": "x"}, "t1", "demo", 2)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("expected lose claim")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMockAppendSlotConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectDDL(mock)

	nl, err := OpenDB(db)
	if err != nil {
		t.Fatal(err)
	}

	step := 1
	mock.ExpectBegin()
	mock.ExpectExec("SAVEPOINT insert_node").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO nodes").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id FROM nodes").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("other-node-id"))

	_, err = nl.Append("DECISION", &step, map[string]any{"plan": "b"}, "t1", "demo", 1)
	if !errors.Is(err, ErrSlotConflict) && !strings.Contains(err.Error(), "slot conflict") {
		t.Fatalf("err=%v want slot conflict", err)
	}
}
