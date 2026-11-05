package postgres

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestMockGetNodePayload(t *testing.T) {
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
	mock.ExpectQuery(`SELECT payload_json FROM nodes WHERE tenant_id = \$1 AND trajectory_id = \$2\s+AND COALESCE\(step_n, -1\) = COALESCE\(\$3::integer, -1\) AND seq = \$4 AND kind = \$5 LIMIT 1`).
		WithArgs("demo", "t1", 1, 3, "TOOL_RESULT").
		WillReturnRows(sqlmock.NewRows([]string{"payload_json"}).AddRow(`{"result":"ok"}`))
	got, found, err := nl.GetNodePayload("t1", "demo", &step, 3, "TOOL_RESULT")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got["result"] != "ok" {
		t.Fatalf("payload=%#v", got)
	}

	mock.ExpectQuery(`SELECT payload_json FROM nodes`).
		WithArgs("demo", "t1", 1, 5, "TOOL_RESULT").
		WillReturnRows(sqlmock.NewRows([]string{"payload_json"}))
	_, found, err = nl.GetNodePayload("t1", "demo", &step, 5, "TOOL_RESULT")
	if err != nil || found {
		t.Fatalf("empty slot: found=%v err=%v", found, err)
	}

	mock.ExpectQuery(`SELECT payload_json FROM nodes`).
		WithArgs("demo", "t1", nil, 0, "STATE_SET").
		WillReturnRows(sqlmock.NewRows([]string{"payload_json"}).AddRow(`{"k":"v"}`))
	got, found, err = nl.GetNodePayload("t1", "demo", nil, 0, "STATE_SET")
	if err != nil || !found || got["k"] != "v" {
		t.Fatalf("nil step: got=%#v found=%v err=%v", got, found, err)
	}

	if _, _, err := nl.GetNodePayload("t1", "", &step, 3, "TOOL_RESULT"); err == nil {
		t.Fatal("empty tenant must be rejected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
