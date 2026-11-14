package console

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSummarizeSealsKeepsOrderAndFailedReason(t *testing.T) {
	t.Parallel()
	events := []Event{
		sealEvent("n", KindNodeAppended, `{"node_id":"n0","kind":"TOOL_CALL","step_n":1}`),
		sealEvent("c1", KindSealCreated, `{"node_id":"n1","step_n":1,"content_hash":"hash-one","tool_names":["echo","ship_release"]}`),
		sealEvent("c2", KindSealCreated, `{"node_id":"n2","step_n":2,"content_hash":"hash-two"}`),
		sealEvent("vbad", KindSealVerified, `{"node_id":"n2","ok":false,"content_hash":"hash-two","error":"hash mismatch"}`),
		sealEvent("vok", KindSealVerified, `{"node_id":"n1","ok":true,"content_hash":"hash-one"}`),
		sealEvent("vmissing", KindSealVerified, `{"node_id":"n3","content_hash":"hash-three"}`),
	}
	sum := Summarize("seals", events)
	if sum.NodeCount != 1 || sum.SealCreatedCount != 2 || sum.SealVerifiedOK != 1 || sum.SealVerifiedFail != 2 {
		t.Fatalf("counts nodes=%d created=%d ok=%d fail=%d", sum.NodeCount, sum.SealCreatedCount, sum.SealVerifiedOK, sum.SealVerifiedFail)
	}
	rows := sum.Seals.Rows
	if len(rows) != 5 {
		t.Fatalf("len=%d %+v", len(rows), rows)
	}
	if rows[0].Status != sealStatusCreated || rows[0].NodeID != "n1" || rows[0].StepN != 1 || rows[0].ContentHash != "hash-one" {
		t.Fatalf("first %+v", rows[0])
	}
	if len(rows[0].ToolNames) != 2 || rows[0].ToolNames[0] != "echo" || rows[0].ToolNames[1] != "ship_release" {
		t.Fatalf("tools %+v", rows[0].ToolNames)
	}
	if rows[1].ID != "c2" || rows[1].Kind != KindSealCreated || rows[1].Status != sealStatusCreated {
		t.Fatalf("second %+v", rows[1])
	}
	if rows[2].Status != sealStatusFailed || rows[2].Error != "hash mismatch" || rows[2].NodeID != "n2" || rows[2].ContentHash != "hash-two" {
		t.Fatalf("failed %+v", rows[2])
	}
	if rows[3].Status != sealStatusVerified || rows[3].Error != "" || rows[3].ContentHash != "hash-one" {
		t.Fatalf("verified %+v", rows[3])
	}
	if rows[4].Status != sealStatusFailed || rows[4].NodeID != "n3" {
		t.Fatalf("missing ok %+v", rows[4])
	}
}

func TestSummarizeSealsEmptyWhenStepsHaveNoLock(t *testing.T) {
	t.Parallel()
	sum := Summarize("plain", []Event{
		sealEvent("n", KindNodeAppended, `{"node_id":"n0","kind":"TOOL_CALL"}`),
	})
	if sum.NodeCount != 1 {
		t.Fatalf("nodes=%d", sum.NodeCount)
	}
	if sum.Seals.Rows == nil || len(sum.Seals.Rows) != 0 {
		t.Fatalf("rows %+v", sum.Seals.Rows)
	}
	raw, err := json.Marshal(sum.Seals)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"rows":[]}` {
		t.Fatalf("json %s", raw)
	}
}

func TestSealsUITrustsServerRows(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "function renderSeals")
	end := strings.Index(src, "function renderTransfers")
	if start < 0 || end < start {
		t.Fatal("renderSeals block missing")
	}
	fn := src[start:end]
	for _, want := range []string{
		"s.seals",
		"content_hash",
		"node_id",
		"tool_names",
		"row.error",
		`status === "failed"`,
		"This run has steps, but no decision is locked yet.",
		"No lock events in this time range.",
	} {
		if !strings.Contains(fn, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, ban := range []string{"filteredEvents", "e.payload", "payload.ok", "crypto", "sha256", "createHash"} {
		if strings.Contains(fn, ban) {
			t.Fatalf("renderSeals uses %q", ban)
		}
	}
}

func sealEvent(id, kind, payload string) Event {
	return Event{
		SchemaVersion: SchemaVersion,
		ID:            id,
		TS:            "2026-09-22T15:00:00Z",
		Kind:          kind,
		Source:        "go",
		TrajectoryID:  "seals",
		Payload:       json.RawMessage(payload),
	}
}
