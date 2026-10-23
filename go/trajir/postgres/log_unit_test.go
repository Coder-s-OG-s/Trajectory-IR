package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
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
	payload := map[string]any{"plan": "x"}
	want, err := nodes.NewNode("DECISION", "t1", "demo", &step, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("SAVEPOINT insert_node").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO nodes").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT id FROM nodes").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(want.ID))
	mock.ExpectCommit()

	n, err := nl.Append("DECISION", &step, payload, "t1", "demo", 1)
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

func TestListNodesKeepsIntegerLiteral(t *testing.T) {
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
	mock.ExpectQuery("SELECT id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "trajectory_id", "tenant_id", "step_n", "seq", "kind", "payload_json", "ts",
		}).AddRow("id-1", "t1", "demo", 1, 1, "TOOL_RESULT", `{"n":9007199254740993}`, 1.5))
	rows, err := nl.ListNodes("t1", "demo")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := rows[0]["payload"].(map[string]any)
	num, ok := payload["n"].(json.Number)
	if !ok || num != "9007199254740993" {
		t.Fatalf("n=%#v", payload["n"])
	}
	if _, err := nodes.PayloadHash(payload); err == nil {
		t.Fatal("hashed a rounded integer")
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

func TestMockClaimToolCallInsertUniqueViolationReturnsFalse(t *testing.T) {
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
	mock.ExpectExec("INSERT INTO nodes").WillReturnError(&pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint \"idx_nodes_slot\""})
	mock.ExpectRollback()

	claimed, err := nl.ClaimToolCall(1, map[string]any{"tool": "x"}, "t1", "demo", 2)
	if err != nil {
		t.Fatalf("unexpected err on unique violation: %v", err)
	}
	if claimed {
		t.Fatal("expected lose claim on unique violation")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMockClaimToolCallInsertNonConflictErrorPropagates(t *testing.T) {
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
	mock.ExpectExec("INSERT INTO nodes").WillReturnError(errors.New("connection reset by peer"))
	mock.ExpectRollback()

	claimed, err := nl.ClaimToolCall(1, map[string]any{"tool": "x"}, "t1", "demo", 2)
	if err == nil {
		t.Fatal("expected non-conflict error to be propagated, got nil")
	}
	if !strings.Contains(err.Error(), "connection reset by peer") {
		t.Fatalf("expected error to mention 'connection reset by peer', got %v", err)
	}
	if claimed {
		t.Fatal("expected claimed=false on error")
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

func TestMockAppendEmptyOwner(t *testing.T) {
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
	mock.ExpectQuery("SELECT id FROM nodes").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err = nl.Append("DECISION", &step, map[string]any{"plan": "x"}, "t1", "demo", 1)
	if !errors.Is(err, ErrNotStored) {
		t.Fatalf("err=%v want ErrNotStored", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMockAppendInsertErrorKeepsTheDriverError(t *testing.T) {
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
	mock.ExpectExec("INSERT INTO nodes").WillReturnError(errors.New("insert failed"))
	mock.ExpectExec("ROLLBACK TO SAVEPOINT insert_node").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id FROM nodes").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err = nl.Append("DECISION", &step, map[string]any{"plan": "x"}, "t1", "demo", 1)
	if !errors.Is(err, ErrNotStored) || !strings.Contains(err.Error(), "insert failed") {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMockAppendSlotLookupErrorIsReturned(t *testing.T) {
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
	mock.ExpectExec("INSERT INTO nodes").WillReturnError(errors.New("insert failed"))
	mock.ExpectExec("ROLLBACK TO SAVEPOINT insert_node").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT id FROM nodes").WillReturnError(errors.New("connection reset"))
	mock.ExpectRollback()

	_, err = nl.Append("DECISION", &step, map[string]any{"plan": "x"}, "t1", "demo", 1)
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSanitizePGErrRedactsPassword(t *testing.T) {
	rawDSN := "postgres://user:super_secret_password123@localhost:5432/trajir_db"
	errWithSecret := errors.New("failed to connect to server: super_secret_password123 was rejected")
	sanitized := sanitizePGErr(errWithSecret, rawDSN)
	if strings.Contains(sanitized.Error(), "super_secret_password123") {
		t.Fatalf("sanitized error leaked password: %v", sanitized)
	}
	if !strings.Contains(sanitized.Error(), "[REDACTED]") {
		t.Fatalf("expected [REDACTED] in error, got: %v", sanitized)
	}

	rawKV := "host=localhost user=admin password=another_secret dbname=db"
	errWithKV := errors.New("connection failed: password=another_secret invalid")
	sanitizedKV := sanitizePGErr(errWithKV, rawKV)
	if strings.Contains(sanitizedKV.Error(), "another_secret") {
		t.Fatalf("sanitized KV leaked password: %v", sanitizedKV)
	}
}

func TestSanitizePGErrQuotedAndURLPasswords(t *testing.T) {
	cases := []struct {
		name, dsn, msg string
		leaks          []string
	}{
		{"quoted kv with space", "host=h user=u password='my secret' dbname=d", "bad dsn: password='my secret' rejected", []string{"my secret", "secret'"}},
		{"quoted kv escaped quote", `host=h password='it\'s a pw' dbname=d`, `bad dsn: password='it\'s a pw' x`, []string{`it\'s`, "a pw"}},
		{"spaces around equals", "host=h password = hunter22xx dbname=d", "bad: password = hunter22xx", []string{"hunter22xx"}},
		{"url userinfo", "postgres://u:pa55w0rd-long@h/d", "dial postgres://u:pa55w0rd-long@h/d failed", []string{"pa55w0rd-long"}},
		{"literal in message", "postgres://u:abcdefghij@h/d", "server said abcdefghij is wrong", []string{"abcdefghij"}},
	}
	for _, tc := range cases {
		got := sanitizePGErr(errors.New(tc.msg), tc.dsn).Error()
		for _, leak := range tc.leaks {
			if strings.Contains(got, leak) {
				t.Errorf("%s: leaked %q in %q", tc.name, leak, got)
			}
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("%s: expected [REDACTED] in %q", tc.name, got)
		}
	}
}

func TestSanitizePGErrShortPasswordDoesNotMangleMessage(t *testing.T) {
	for _, pw := range []string{"a", "5", "ab"} {
		dsn := "postgres://user:" + pw + "@db.example.com:5432/app"
		msg := "dial tcp db.example.com:5432: connection refused"
		if got := sanitizePGErr(errors.New(msg), dsn).Error(); got != msg {
			t.Errorf("password %q mangled unrelated text: %q", pw, got)
		}
		// The password itself, as a whole token, is still scrubbed.
		got := sanitizePGErr(errors.New("auth failed for "+pw+" now"), dsn).Error()
		if strings.Contains(got, " "+pw+" ") {
			t.Errorf("password %q not scrubbed as a token: %q", pw, got)
		}
	}
}

func TestSanitizePGErrKeepsErrorChain(t *testing.T) {
	cause := fmt.Errorf("ping: %w", context.DeadlineExceeded)
	err := sanitizePGErr(fmt.Errorf("failed: %w, password=topsecret1", cause), "host=h password=topsecret1")
	if strings.Contains(err.Error(), "topsecret1") {
		t.Fatalf("leaked password: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(DeadlineExceeded) = false for %v", err)
	}
	if sanitizePGErr(nil, "x") != nil {
		t.Fatal("nil must stay nil")
	}
	plain := errors.New("nothing secret here")
	if sanitizePGErr(plain, "host=h password=zzzzzzzzzz") != plain {
		t.Fatal("an unchanged error must be returned as is")
	}
}

func TestOpenDSNRedactsPasswordOnEarlyErrors(t *testing.T) {
	t.Setenv("TRAJIR_PG_CONNECT_TIMEOUT", "")
	_, err := OpenDSN("postgres://user:Sup3rS3cret@bad host/db?connect_timeout=oops")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "Sup3rS3cret") {
		t.Fatalf("leaked password: %v", err)
	}
}
