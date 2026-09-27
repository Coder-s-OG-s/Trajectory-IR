package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMultiSealFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "multi_seal.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(st.Root(), "trajectories", "demo-seals.ndjson")
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(st, "")
	req := httptest.NewRequest(http.MethodGet, "/v1/trajectories/demo-seals/summary", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var sum Summary
	if err := json.Unmarshal(rr.Body.Bytes(), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.NodeCount != 7 || sum.SealCreatedCount != 3 || sum.SealVerifiedOK != 1 || sum.SealVerifiedFail != 1 {
		t.Fatalf("counts %+v", sum)
	}
	if len(sum.Seals) != 5 {
		t.Fatalf("seals=%d", len(sum.Seals))
	}

	first := sum.Seals[0]
	if first.ID != "e4" || first.Status != statusCreated || first.Kind != KindSealCreated || first.Chain != chainContinuous {
		t.Fatalf("first %+v", first)
	}
	if first.StepN == nil || *first.StepN != 3 || first.ContentHash != "aaa111" {
		t.Fatalf("first hash %+v", first)
	}
	if first.FromSeq == nil || first.ToSeq == nil || *first.FromSeq != 1 || *first.ToSeq != 3 || first.CoveredNodes != 3 {
		t.Fatalf("first range %+v", first)
	}
	if first.FromNodeID != "n1" || first.ToNodeID != "d1" || len(first.ToolNames) != 1 || first.ToolNames[0] != "deploy" {
		t.Fatalf("first linkage %+v", first)
	}

	verified := sum.Seals[1]
	if verified.Status != statusVerified || verified.Chain != chainContinuous || verified.Error != "" {
		t.Fatalf("verified %+v", verified)
	}
	if verified.FromSeq == nil || *verified.FromSeq != 1 || verified.CoveredNodes != 3 {
		t.Fatalf("verified range %+v", verified)
	}

	failed := sum.Seals[3]
	if failed.ID != "e9" || failed.Status != statusFailed || failed.Error != "content hash mismatch" {
		t.Fatalf("failed %+v", failed)
	}
	if failed.Kind != KindSealVerified || failed.Chain != chainContinuous || failed.ContentHash != "bbb222" {
		t.Fatalf("failed linkage %+v", failed)
	}

	open := sum.Seals[4]
	if open.Status != statusCreated || open.NodeID != "d3" || open.CoveredNodes != 2 {
		t.Fatalf("open %+v", open)
	}
	if open.FromSeq == nil || open.ToSeq == nil || *open.FromSeq != 6 || *open.ToSeq != 7 {
		t.Fatalf("open range %+v", open)
	}

	for i := 1; i < len(sum.Seals); i++ {
		if sum.Seals[i].TS < sum.Seals[i-1].TS {
			t.Fatalf("not chronological at %d", i)
		}
	}
}

func TestSealsEmptyWhenNodesHaveNoSeals(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{
		{
			SchemaVersion: SchemaVersion, ID: "n", TS: ts, Kind: KindNodeAppended, Source: "go",
			TrajectoryID: "bare", Payload: json.RawMessage(`{"node_id":"n1","kind":"TOOL_CALL","seq":1}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "n2", TS: ts, Kind: KindNodeAppended, Source: "go",
			TrajectoryID: "bare", Payload: json.RawMessage(`{"node_id":"n2","kind":"TOOL_RESULT","seq":2}`),
		},
	}
	sum := Summarize("bare", events)
	if sum.NodeCount != 2 || len(sum.Seals) != 0 || sum.SealCreatedCount != 0 {
		t.Fatalf("%+v", sum)
	}
}

func TestSealChainBreaks(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)
	mk := func(id, kind, payload string) Event {
		return Event{
			SchemaVersion: SchemaVersion, ID: id, TS: ts, Kind: kind, Source: "cli",
			TrajectoryID: "t", Payload: json.RawMessage(payload),
		}
	}
	sum := Summarize("t", []Event{
		mk("z", KindNodeAppended, `{"node_id":"n0","kind":"THOUGHT"}`),
		mk("a", KindSealCreated, `{"node_id":"d1","step_n":4,"content_hash":"h1"}`),
		mk("b", KindSealCreated, `{"node_id":"d2","step_n":4,"content_hash":"h2"}`),
		mk("c", KindSealVerified, `{"node_id":"missing","ok":true,"content_hash":"h3"}`),
		mk("d", KindSealVerified, `{"node_id":"d9","ok":false,"content_hash":"h4"}`),
	})
	if sum.Seals[0].Chain != chainContinuous || sum.Seals[1].Chain != chainBreak {
		t.Fatalf("created chain %+v %+v", sum.Seals[0], sum.Seals[1])
	}
	if sum.Seals[0].CoveredNodes != 1 || sum.Seals[0].FromSeq != nil || sum.Seals[0].FromNodeID != "n0" {
		t.Fatalf("unsequenced span %+v", sum.Seals[0])
	}
	if sum.Seals[2].Status != statusVerified || sum.Seals[2].Chain != chainBreak {
		t.Fatalf("orphan verify %+v", sum.Seals[2])
	}
	if sum.Seals[3].Status != statusFailed || sum.Seals[3].Error != "" || sum.Seals[3].Chain != chainBreak {
		t.Fatalf("failed without reason %+v", sum.Seals[3])
	}
}

func TestSealsUITrustsServerStatus(t *testing.T) {
	t.Parallel()
	jsb, err := UI.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(jsb)
	start := strings.Index(js, "function renderSeals")
	end := strings.Index(js, "function renderTransfers")
	if start < 0 || end < start {
		t.Fatal("renderSeals block missing")
	}
	body := js[start:end]
	for _, want := range []string{"s.seals", "row.status", "row.error", "no seals yet", "seal-failed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("seals panel missing %q", want)
		}
	}
	for _, banned := range []string{"crypto", "sha256", "SHA-256", "p.ok", "payload.ok"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(banned)) {
			t.Fatalf("seals panel must not derive integrity itself: %s", banned)
		}
	}
	css, err := UI.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	sheet := string(css)
	if !strings.Contains(sheet, ".seal-failed") || !strings.Contains(sheet, ".badge.failed") {
		t.Fatal("failed verify is not visually distinct")
	}
}
