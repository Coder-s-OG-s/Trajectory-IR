package emit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

func TestEnvelopeFields(t *testing.T) {
	var got emit.Event
	sink := emit.SinkFunc(func(e emit.Event) error {
		got = e
		return nil
	})
	emit.SafeEmit(sink, emit.Event{
		Kind:         emit.KindNodeAppended,
		TrajectoryID: "t1",
		TenantID:     "demo",
		Runtime:      "go",
		Payload: map[string]any{
			"node_id":      "n1",
			"kind":         "DECISION",
			"content_hash": "abc",
		},
	})
	if got.SchemaVersion != emit.SchemaVersion || got.Kind != "node.appended" {
		t.Fatalf("envelope %+v", got)
	}
	if got.ID == "" || got.TS == "" || got.Source != "go" {
		t.Fatalf("defaults %+v", got)
	}
	for _, key := range []string{"node_id", "kind", "content_hash"} {
		if _, ok := got.Payload[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
}

func TestSafeEmitIgnoresFailureAndPanic(t *testing.T) {
	emit.SafeEmit(emit.SinkFunc(func(emit.Event) error {
		return os.ErrClosed
	}), emit.Event{Kind: emit.KindSealCreated, TrajectoryID: "t", Payload: map[string]any{"node_id": "n"}})
	emit.SafeEmit(emit.SinkFunc(func(emit.Event) error {
		panic("disk")
	}), emit.Event{Kind: emit.KindSealCreated, TrajectoryID: "t", Payload: map[string]any{"node_id": "n"}})
}

func TestFileSinkLayout(t *testing.T) {
	dir := t.TempDir()
	sink := &emit.FileSink{Dir: dir}
	emit.SafeEmit(sink, emit.Event{
		Kind:         emit.KindExportCompleted,
		TrajectoryID: "pack",
		Payload: map[string]any{
			"path": "out.tir", "mode": "thin", "redacted": true,
			"bytes": 10, "member_count": 5, "node_count": 1, "ok": true,
		},
	})
	b, err := os.ReadFile(filepath.Join(dir, "trajectories", "pack.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	var ev emit.Event
	if err := json.Unmarshal(b, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Kind != "export.completed" || ev.Payload["ok"] != true {
		t.Fatalf("%+v", ev)
	}
	if err := sink.Emit(emit.Event{Kind: "node.appended", TrajectoryID: "../nope"}); err == nil {
		t.Fatal("expected bad trajectory id")
	}
}

func TestHTTPSinkStatus(t *testing.T) {
	var auth string
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" || r.Method != http.MethodPost {
			t.Errorf("path %s %s", r.Method, r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer okSrv.Close()
	sink := &emit.HTTPSink{BaseURL: okSrv.URL, Token: "dev-token", Client: okSrv.Client()}
	if err := sink.Emit(emit.Event{Kind: emit.KindNodeAppended, TrajectoryID: "t", Payload: map[string]any{"node_id": "n", "kind": "COMMIT_STEP"}}); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer dev-token" {
		t.Fatalf("auth %q", auth)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	fail := &emit.HTTPSink{BaseURL: bad.URL, Client: bad.Client()}
	if err := fail.Emit(emit.Event{Kind: emit.KindNodeAppended, TrajectoryID: "t"}); err == nil {
		t.Fatal("expected non-201")
	}
}

func TestNotePackageImportFlag(t *testing.T) {
	var got emit.Event
	ok := true
	redacted := false
	emit.NotePackage(emit.SinkFunc(func(e emit.Event) error {
		got = e
		return nil
	}), emit.PackageFact{
		Kind:         emit.KindImportCompleted,
		TrajectoryID: "t",
		Path:         "in.tir",
		Mode:         "thin",
		Redacted:     &redacted,
		Bytes:        4,
		MemberCount:  5,
		NodeCount:    1,
		OK:           &ok,
		Source:       "go",
	})
	if got.Kind != "import.completed" || got.Payload["verify_ok"] != true {
		t.Fatalf("%+v", got)
	}
	if _, isExportOK := got.Payload["ok"]; isExportOK {
		t.Fatal("import used ok")
	}
}
