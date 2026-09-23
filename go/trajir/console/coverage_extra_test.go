package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsKnownKindAndRoot(t *testing.T) {
	t.Parallel()
	if !IsKnownKind(KindSealCreated) {
		t.Fatal("expected known kind")
	}
	if IsKnownKind("nope.kind") {
		t.Fatal("expected unknown")
	}
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if st.Root() == "" {
		t.Fatal("root empty")
	}
}

func TestOpenStoreRequiresDir(t *testing.T) {
	t.Setenv(envDataDir, "")
	if _, err := OpenStore(""); err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenStoreFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envDataDir, dir)
	st, err := OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Root(), trajectoriesDir)); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPListAndEvents(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := sampleEvent(KindNodeAppended)
	e.Payload = json.RawMessage(`{"node_id":"n1","kind":"TOOL_CALL"}`)
	if err := st.Append(e); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")

	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/trajectories", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var list struct {
		Trajectories []string `json:"trajectories"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Trajectories) != 1 || list.Trajectories[0] != "t-demo" {
		t.Fatalf("list=%v", list.Trajectories)
	}

	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/v1/trajectories/t-demo/events", nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("events status=%d", rr2.Code)
	}

	rr3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr3, httptest.NewRequest(http.MethodGet, "/v1/trajectories/missing/events", nil))
	if rr3.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", rr3.Code)
	}

	rr4 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr4, httptest.NewRequest(http.MethodGet, "/v1/trajectories/missing/summary", nil))
	if rr4.Code != http.StatusNotFound {
		t.Fatalf("summary missing status=%d", rr4.Code)
	}
}

func TestHTTPBodyTooLarge(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")
	huge := bytes.Repeat([]byte("a"), maxBodyBytes+2)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(huge))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestSummarizeSealVerifyFailAndBadPayloadTypes(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 9, 22, 16, 0, 0, 0, time.UTC).Format(time.RFC3339)
	events := []Event{
		{
			SchemaVersion: SchemaVersion, ID: "1", TS: ts, Kind: KindSealVerified, Source: "go",
			TrajectoryID: "t1", Payload: json.RawMessage(`{"node_id":"n","ok":false,"content_hash":"h","error":"mismatch"}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "2", TS: ts, Kind: KindContextProjected, Source: "go",
			TrajectoryID: "t1",
			Payload: json.RawMessage(`{
				"budget":"nope","metric":"rfc8785_bytes","size_units":1,
				"included_ids":"bad","dropped_ids":[1,2],
				"raw_size_units":true
			}`),
		},
		{
			SchemaVersion: SchemaVersion, ID: "3", TS: ts, Kind: KindExportCompleted, Source: "cli",
			TrajectoryID: "t1",
			Payload:      json.RawMessage(`{"path":"x","mode":"thin","redacted":"yes","bytes":"x","member_count":null,"node_count":2,"ok":false}`),
		},
	}
	sum := Summarize("t1", events)
	if sum.SealVerifiedFail != 1 {
		t.Fatalf("fail=%d", sum.SealVerifiedFail)
	}
}

func TestParseNDJSONSkipsBlankAndRejectsBadLine(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Root(), trajectoriesDir, "t-demo.ndjson")
	good := sampleEvent(KindNodeAppended)
	good.Payload = json.RawMessage(`{}`)
	line, _ := json.Marshal(good)
	content := append([]byte("\n\n"), line...)
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := st.ReadEvents("t-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("len=%d", len(events))
	}

	if err := os.WriteFile(path, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReadEvents("t-demo"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestValidateNilAndLongTrajectoryID(t *testing.T) {
	t.Parallel()
	var e *Event
	if err := e.Validate(); err == nil {
		t.Fatal("nil")
	}
	ev := sampleEvent(KindNodeAppended)
	ev.TrajectoryID = string(bytes.Repeat([]byte("x"), 201))
	if err := ev.Validate(); err == nil {
		t.Fatal("expected long id error")
	}
}

func TestListTrajectoriesSkipsJunkFiles(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(st.Root(), trajectoriesDir)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	_ = os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "..bad.ndjson"), []byte("{}\n"), 0o644)
	ids, err := st.ListTrajectories()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids=%v", ids)
	}
}

func TestListenAndServeInvalidAddr(t *testing.T) {
	t.Parallel()
	err := ListenAndServe("127.0.0.1:999999", http.NewServeMux())
	if err == nil {
		t.Fatal("expected listen error")
	}
}
