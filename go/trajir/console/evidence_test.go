package console

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDeriveEvidenceSealBeforeExecuteOK(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{
		{
			SchemaVersion: SchemaVersion, ID: "1", TS: ts, Kind: KindSealCreated, Source: "python",
			TrajectoryID: "ev", Payload: json.RawMessage(`{"node_id":"d1","step_n":1,"content_hash":"h","tool_names":["echo"]}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "2", TS: ts, Kind: KindNodeAppended, Source: "python",
			TrajectoryID: "ev",
			Payload:      json.RawMessage(`{"node_id":"d1","kind":"DECISION","seq":1,"step_n":1,"content_hash":"h"}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "3", TS: ts, Kind: KindNodeAppended, Source: "python",
			TrajectoryID: "ev",
			Payload: json.RawMessage(`{
				"node_id":"t1","kind":"TOOL_CALL","seq":2,"step_n":1,
				"tool":"echo","idempotency_key":"ik","effect_class":"PURE","content_hash":"th"
			}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "4", TS: ts, Kind: KindAuditCompleted, Source: "cli",
			TrajectoryID: "ev",
			Payload: json.RawMessage(`{
				"ok":true,"path":"out.tir","findings":[]
			}`),
		},
	}
	view := deriveEvidence(events)
	if view.SealCount != 1 {
		t.Fatalf("seal_count=%d", view.SealCount)
	}
	if view.SealBeforeExecuteOK == nil || !*view.SealBeforeExecuteOK {
		t.Fatalf("expected seal_before_execute_ok true, gaps=%v", view.SealBeforeExecuteGaps)
	}
	if len(view.ToolCalls) != 1 || view.ToolCalls[0].ToolName != "echo" {
		t.Fatalf("tools %+v", view.ToolCalls)
	}
	if view.ToolCalls[0].IdempotencyKey != "ik" || view.ToolCalls[0].EffectClass != "PURE" {
		t.Fatalf("tool detail %+v", view.ToolCalls[0])
	}
	if view.AuditOK == nil || !*view.AuditOK {
		t.Fatalf("audit %+v", view.AuditOK)
	}
	if view.LastAuditPath != "out.tir" {
		t.Fatalf("path %q", view.LastAuditPath)
	}
}

func TestDeriveEvidenceGapWhenToolBeforeDecision(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{
		{
			SchemaVersion: SchemaVersion, ID: "1", TS: ts, Kind: KindNodeAppended, Source: "python",
			TrajectoryID: "ev",
			Payload: json.RawMessage(`{
				"node_id":"t1","kind":"TOOL_CALL","seq":2,"step_n":1,"tool":"echo"
			}`),
		},
	}
	view := deriveEvidence(events)
	if view.SealBeforeExecuteOK == nil || *view.SealBeforeExecuteOK {
		t.Fatalf("expected gap, ok=%v gaps=%v", view.SealBeforeExecuteOK, view.SealBeforeExecuteGaps)
	}
	if len(view.SealBeforeExecuteGaps) != 1 {
		t.Fatalf("gaps %+v", view.SealBeforeExecuteGaps)
	}
}

func TestDeriveEvidenceOpenWorldAndAuditFindings(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{
		{
			SchemaVersion: SchemaVersion, ID: "1", TS: ts, Kind: KindSealCreated, Source: "python",
			TrajectoryID: "ev",
			Payload:      json.RawMessage(`{"node_id":"d1","step_n":1,"content_hash":"h","tool_names":["bash"]}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "2", TS: ts, Kind: KindNodeAppended, Source: "python",
			TrajectoryID: "ev",
			Payload: json.RawMessage(`{
				"node_id":"t1","kind":"TOOL_CALL","seq":2,"step_n":1,"tool":"bash","effect_class":"NON_IDEMPOTENT_WRITE"
			}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "3", TS: ts, Kind: KindAuditCompleted, Source: "cli",
			TrajectoryID: "ev",
			Payload: json.RawMessage(`{
				"ok":false,"path":"bad.tir",
				"findings":[{"code":"SEAL_BEFORE_EXECUTE","message":"tool before decision"}]
			}`),
		},
	}
	view := deriveEvidence(events)
	if len(view.OpenWorldTools) != 1 || view.OpenWorldTools[0] != "bash" {
		t.Fatalf("open_world %+v", view.OpenWorldTools)
	}
	if view.ToolCalls[0].OpenWorld != true {
		t.Fatalf("tool open_world flag %+v", view.ToolCalls[0])
	}
	if view.AuditOK == nil || *view.AuditOK {
		t.Fatalf("expected audit fail")
	}
	if len(view.AuditFindings) != 1 || view.AuditFindings[0] == "" {
		t.Fatalf("findings %+v", view.AuditFindings)
	}
}

func TestSummarizeIncludesEvidence(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{{
		SchemaVersion: SchemaVersion, ID: "1", TS: ts, Kind: KindSealCreated, Source: "go",
		TrajectoryID: "t1", Payload: json.RawMessage(`{"node_id":"n","step_n":1,"content_hash":"h"}`),
	}}
	sum := Summarize("t1", events)
	if sum.Evidence.SealCount != 1 {
		t.Fatalf("evidence %+v", sum.Evidence)
	}
	if sum.Evidence.SealBeforeExecuteOK == nil || !*sum.Evidence.SealBeforeExecuteOK {
		t.Fatalf("expected ok with seals and no tool gaps")
	}
}

func TestIsKnownKindAuditCompleted(t *testing.T) {
	t.Parallel()
	if !IsKnownKind(KindAuditCompleted) {
		t.Fatal("audit.completed should be known")
	}
}
