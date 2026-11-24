package nodelog_test

import (
	"reflect"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

func TestListTrajectoriesCountsNodesAndSeals(t *testing.T) {
	nl := openTemp(t)
	add := func(traj, tenant, kind string, step int) {
		t.Helper()
		if _, err := nl.Append(kind, &step, map[string]any{"s": step}, traj, tenant, map[string]int{"PROJECT_CONTEXT": 0, "DECISION": 1}[kind]); err != nil {
			t.Fatal(err)
		}
	}
	add("b", "local", "PROJECT_CONTEXT", 1)
	add("b", "local", "DECISION", 1)
	add("b", "local", "DECISION", 2)
	add("a", "team", "PROJECT_CONTEXT", 1)
	got, err := nl.ListTrajectories()
	if err != nil {
		t.Fatal(err)
	}
	want := []nodelog.TrajectorySummary{
		{TrajectoryID: "a", TenantID: "team", NodeCount: 1, SealCount: 0},
		{TrajectoryID: "b", TenantID: "local", NodeCount: 3, SealCount: 2},
	}
	if len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	for i := range got {
		got[i].LastTS = 0
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
