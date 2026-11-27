package nodelog_test

import (
	"testing"
)

func TestListStepNodesReturnsOneStepInSeqOrder(t *testing.T) {
	nl := openTemp(t)
	for step := 1; step <= 2; step++ {
		s := step
		for _, kind := range []string{"PROJECT_CONTEXT", "COMMIT_STEP"} {
			seq := 0
			if kind == "COMMIT_STEP" {
				seq = 4
			}
			if _, err := nl.Append(kind, &s, map[string]any{"step": s}, "traj", "local", seq); err != nil {
				t.Fatal(err)
			}
		}
	}
	other := 2
	if _, err := nl.Append("PROJECT_CONTEXT", &other, map[string]any{}, "traj", "other-tenant", 0); err != nil {
		t.Fatal(err)
	}
	rows, err := nl.ListStepNodes("traj", "local", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %v", rows)
	}
	if rows[0]["kind"] != "PROJECT_CONTEXT" || rows[1]["kind"] != "COMMIT_STEP" {
		t.Fatalf("order = %v, %v", rows[0]["kind"], rows[1]["kind"])
	}
	for _, r := range rows {
		if r["step_n"] != 2 || r["tenant_id"] != "local" {
			t.Fatalf("row from wrong step or tenant: %v", r)
		}
	}
}
