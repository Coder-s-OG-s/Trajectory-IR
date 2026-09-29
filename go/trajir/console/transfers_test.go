package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadNDJSON(t *testing.T, name string) []Event {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		e, err := ParseEvent(line)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTransfersHandoffs(t *testing.T) {
	events := loadNDJSON(t, "transfers.ndjson")
	sum := Summarize("xfer", events)
	hs := sum.Transfers.Handoffs
	if len(hs) != 5 {
		t.Fatalf("handoffs=%d %+v", len(hs), hs)
	}
	if sum.Transfers.ExportsOK != 3 || sum.Transfers.ImportsOK != 2 {
		t.Fatalf("counts %+v", sum.Transfers)
	}

	if hs[0].Status != handoffImportOnly || hs[0].ImportSource != "derived" {
		t.Fatalf("orphan %+v", hs[0])
	}
	if hs[0].VerifyOK == nil || !*hs[0].VerifyOK || hs[0].ImportPath != "orphan.tir" {
		t.Fatalf("orphan verify %+v", hs[0])
	}

	if hs[1].Status != handoffConnected || hs[1].ExportPath != "shared.tir" || hs[1].ImportPath != "shared.tir" {
		t.Fatalf("connected %+v", hs[1])
	}
	if hs[1].Redacted == nil || !*hs[1].Redacted {
		t.Fatalf("redacted label %+v", hs[1])
	}
	if hs[1].ExportSource != "go" || hs[1].ImportSource != "python" || hs[1].Bytes != 1652 {
		t.Fatalf("connected ends %+v", hs[1])
	}

	if hs[2].Status != handoffExportOnly || hs[2].ExportPath != "first.tir" {
		t.Fatalf("open export %+v", hs[2])
	}

	if hs[3].Status != handoffFailed || hs[3].Error != "HashMismatch" || hs[3].ExportPath != "second.tir" {
		t.Fatalf("verify fail %+v", hs[3])
	}
	if hs[3].VerifyOK == nil || *hs[3].VerifyOK {
		t.Fatalf("verify flag %+v", hs[3])
	}

	if hs[4].Status != handoffFailed || hs[4].Error != "WriteError" || hs[4].ExportPath != "bad.tir" {
		t.Fatalf("export fail %+v", hs[4])
	}
	if hs[4].Bytes != 0 || hs[4].MemberCount != 0 || hs[4].NodeCount != 0 {
		t.Fatalf("zero package hidden %+v", hs[4])
	}

	raw, err := json.Marshal(sum.Transfers)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("THOUGHT")) {
		t.Fatalf("thought body leaked: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"bytes":0`)) {
		t.Fatalf("zero-byte export dropped: %s", raw)
	}
}

func TestTransfersPathBeatsOldest(t *testing.T) {
	ts := "2026-09-22T15:00:00Z"
	events := []Event{
		pkgEvent("e1", KindExportCompleted, ts, `{"path":"a.tir","mode":"thin","redacted":false,"bytes":1,"member_count":5,"node_count":1,"ok":true}`),
		pkgEvent("e2", KindExportCompleted, ts, `{"path":"b.tir","mode":"thin","redacted":false,"bytes":2,"member_count":5,"node_count":1,"ok":true}`),
		pkgEvent("i2", KindImportCompleted, ts, `{"path":"b.tir","mode":"thin","bytes":2,"member_count":5,"node_count":1,"verify_ok":true}`),
		pkgEvent("iz", KindImportCompleted, ts, `{"path":"z.tir","mode":"thin","bytes":3,"member_count":5,"node_count":1,"verify_ok":true}`),
	}
	hs := Summarize("t", events).Transfers.Handoffs
	if len(hs) != 2 {
		t.Fatalf("len=%d %+v", len(hs), hs)
	}
	if hs[0].Status != handoffConnected || hs[0].ExportPath != "a.tir" || hs[0].ImportPath != "z.tir" {
		t.Fatalf("oldest fallback %+v", hs[0])
	}
	if hs[1].Status != handoffConnected || hs[1].ExportPath != "b.tir" || hs[1].ImportPath != "b.tir" {
		t.Fatalf("path match %+v", hs[1])
	}
}

func pkgEvent(id, kind, ts, payload string) Event {
	return Event{
		SchemaVersion: SchemaVersion,
		ID:            id,
		TS:            ts,
		Kind:          kind,
		Source:        "go",
		TrajectoryID:  "t",
		Payload:       json.RawMessage(payload),
	}
}

func TestTransfersUITrustsServerHandoffs(t *testing.T) {
	b, err := os.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "function renderTransfers")
	if start < 0 {
		t.Fatal("renderTransfers block missing")
	}
	rest := src[start:]
	next := strings.Index(rest[len("function renderTransfers"):], "\n  function ")
	fn := rest
	if next >= 0 {
		fn = rest[:len("function renderTransfers")+next]
	}
	for _, want := range []string{"s.transfers", "handoffs", "redacted", "No transfer handoffs"} {
		if !strings.Contains(fn, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, ban := range []string{"JSON.stringify", "e.payload", "filteredEvents", "thought"} {
		if strings.Contains(fn, ban) {
			t.Fatalf("renderTransfers uses %q", ban)
		}
	}
}
