package console

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSummarizeEconomyAndTransfers(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 9, 22, 15, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{
		{
			SchemaVersion: SchemaVersion, ID: "1", TS: ts, Kind: KindNodeAppended, Source: "go",
			TrajectoryID: "t1", Payload: json.RawMessage(`{"node_id":"n1","kind":"DECISION"}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "2", TS: ts, Kind: KindSealCreated, Source: "go",
			TrajectoryID: "t1", Payload: json.RawMessage(`{"node_id":"n1","step_n":1,"content_hash":"h"}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "3", TS: ts, Kind: KindSealVerified, Source: "go",
			TrajectoryID: "t1", Payload: json.RawMessage(`{"node_id":"n1","ok":true,"content_hash":"h"}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "4", TS: ts, Kind: KindContextProjected, Source: "python",
			TrajectoryID: "t1",
			Payload: json.RawMessage(`{
				"budget":50000,"metric":"rfc8785_bytes","size_units":100,
				"included_ids":["a"],"dropped_ids":["b","c"],
				"raw_size_units":400,"raw_char_len":400,"projected_char_len":100
			}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "5", TS: ts, Kind: KindRedactionApplied, Source: "python",
			TrajectoryID: "t1",
			Payload:      json.RawMessage(`{"mode":"export","thought_collapses":2,"secret_field_hits":3}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "6", TS: ts, Kind: KindExportCompleted, Source: "go",
			TrajectoryID: "t1",
			Payload: json.RawMessage(`{
				"path":"out.tir","mode":"thin","redacted":true,"bytes":1652,
				"member_count":4,"node_count":7,"ok":true
			}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "7", TS: ts, Kind: KindImportCompleted, Source: "go",
			TrajectoryID: "t1",
			Payload: json.RawMessage(`{
				"path":"out.tir","mode":"thin","redacted":true,"bytes":1652,
				"member_count":4,"node_count":7,"verify_ok":true
			}`),
		},
	}

	sum := Summarize("t1", events)
	if sum.NodeCount != 1 || sum.SealCreatedCount != 1 || sum.SealVerifiedOK != 1 {
		t.Fatalf("seals/nodes %+v", sum)
	}
	if sum.NodesDropped != 2 || sum.ProjectionSizeUnits != 100 || sum.ProjectionBudget != 50000 {
		t.Fatalf("projection %+v", sum)
	}
	if sum.RawEstimatedTokens == nil || *sum.RawEstimatedTokens != 100 {
		t.Fatalf("raw tokens %+v", sum.RawEstimatedTokens)
	}
	if sum.ProjectedEstimatedTokens == nil || *sum.ProjectedEstimatedTokens != 25 {
		t.Fatalf("proj tokens %+v", sum.ProjectedEstimatedTokens)
	}
	if sum.TokensAvoidedEstimated == nil || *sum.TokensAvoidedEstimated != 75 {
		t.Fatalf("avoided %+v", sum.TokensAvoidedEstimated)
	}
	if sum.RedactionCollapses != 5 {
		t.Fatalf("redaction=%d", sum.RedactionCollapses)
	}
	if sum.ExportsOK != 1 || sum.ImportsOK != 1 {
		t.Fatalf("xfer counts %+v", sum)
	}
	if sum.LastPackageMode != "thin" || sum.LastPackageBytes != 1652 {
		t.Fatalf("package %+v", sum)
	}
	if sum.TransferVerifyOK == nil || !*sum.TransferVerifyOK {
		t.Fatalf("verify %+v", sum.TransferVerifyOK)
	}
}

func TestSummarizeMissingCharLensNullAvoided(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 9, 22, 15, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{{
		SchemaVersion: SchemaVersion, ID: "1", TS: ts, Kind: KindContextProjected, Source: "go",
		TrajectoryID: "t1",
		Payload: json.RawMessage(`{
			"budget":10,"metric":"rfc8785_bytes","size_units":5,
			"included_ids":[],"dropped_ids":[]
		}`),
	}}
	sum := Summarize("t1", events)
	if sum.TokensAvoidedEstimated != nil || sum.RawEstimatedTokens != nil {
		t.Fatalf("expected null token estimates, got %+v", sum)
	}
}
