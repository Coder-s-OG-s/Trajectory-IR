package projector_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/projector"
)

func node(id, kind string, payload map[string]any, seq int) map[string]any {
	return map[string]any{
		"id":            id,
		"kind":          kind,
		"payload":       payload,
		"step_n":        1,
		"seq":           seq,
		"trajectory_id": "t1",
		"tenant_id":     "demo",
	}
}

func TestConstraintsIncludedUnderBudget(t *testing.T) {
	nodes := []map[string]any{
		node("c1", "CONSTRAINT", map[string]any{"rule": "a"}, 0),
		node("t1", "THOUGHT", map[string]any{"text": "hello"}, 1),
		node("c2", "CONSTRAINT", map[string]any{"rule": "b"}, 2),
	}
	budget := 0
	for _, n := range nodes {
		sz, err := projector.NodeSizeUnits(n)
		if err != nil {
			t.Fatal(err)
		}
		budget += sz
	}
	res, err := projector.ProjectContext(nodes, budget, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, id := range res.IncludedIDs {
		got[id] = true
	}
	if !got["c1"] || !got["c2"] {
		t.Fatalf("missing constraints: %v", res.IncludedIDs)
	}
}

func TestBudgetImpossible(t *testing.T) {
	payload := map[string]any{"rule": strings.Repeat("x", 200)}
	nodes := []map[string]any{
		node("c1", "CONSTRAINT", payload, 0),
		node("c2", "CONSTRAINT", payload, 1),
	}
	must := 0
	for _, n := range nodes {
		sz, err := projector.NodeSizeUnits(n)
		if err != nil {
			t.Fatal(err)
		}
		must += sz
	}
	_, err := projector.ProjectContext(nodes, must-1, nil)
	var bi *projector.BudgetImpossible
	if !errors.As(err, &bi) {
		t.Fatalf("want BudgetImpossible, got %v", err)
	}
	if bi.RequiredSize <= bi.Budget {
		t.Fatalf("required=%d budget=%d", bi.RequiredSize, bi.Budget)
	}
}

func TestTrimOptionalKeepConstraints(t *testing.T) {
	c := node("c1", "CONSTRAINT", map[string]any{"rule": "keep"}, 0)
	thoughts := []map[string]any{
		node("th0", "THOUGHT", map[string]any{"text": strings.Repeat("y", 40)}, 1),
		node("th1", "THOUGHT", map[string]any{"text": strings.Repeat("y", 40)}, 2),
		node("th2", "THOUGHT", map[string]any{"text": strings.Repeat("y", 40)}, 3),
		node("th3", "THOUGHT", map[string]any{"text": strings.Repeat("y", 40)}, 4),
		node("th4", "THOUGHT", map[string]any{"text": strings.Repeat("y", 40)}, 5),
	}
	nodes := append([]map[string]any{c}, thoughts...)
	cSize, err := projector.NodeSizeUnits(c)
	if err != nil {
		t.Fatal(err)
	}
	tSize, err := projector.NodeSizeUnits(thoughts[0])
	if err != nil {
		t.Fatal(err)
	}
	res, err := projector.ProjectContext(nodes, cSize+tSize, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundC := false
	for _, id := range res.IncludedIDs {
		if id == "c1" {
			foundC = true
		}
	}
	if !foundC {
		t.Fatal("constraint dropped")
	}
	if len(res.DroppedIDs) == 0 {
		t.Fatal("expected some thoughts dropped")
	}
	for _, id := range res.DroppedIDs {
		if id == "c1" {
			t.Fatal("constraint in dropped set")
		}
	}
}
