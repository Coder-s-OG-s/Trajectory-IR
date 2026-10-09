package console

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHTTPIngestAndSummary(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "secret")

	e := sampleEvent(KindContextProjected)
	e.Payload = json.RawMessage(`{
		"budget":100,"metric":"rfc8785_bytes","size_units":40,
		"included_ids":["a"],"dropped_ids":["b"],
		"raw_char_len":200,"projected_char_len":80
	}`)
	body, _ := json.Marshal(e)

	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	// reject bad auth
	req2 := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", rr2.Code)
	}

	// reject malformed
	bad := []byte(`{"schema_version":"console-events-v1"}`)
	req3 := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(bad))
	req3.Header.Set("Authorization", "Bearer secret")
	rr3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d", rr3.Code)
	}

	req4 := httptest.NewRequest(http.MethodGet, "/v1/trajectories/t-demo/summary", nil)
	req4.Header.Set("Authorization", "Bearer secret")
	rr4 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", rr4.Code, rr4.Body.String())
	}
	var sum Summary
	if err := json.Unmarshal(rr4.Body.Bytes(), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.TokensAvoidedEstimated == nil || *sum.TokensAvoidedEstimated != 30 {
		t.Fatalf("avoided=%v", sum.TokensAvoidedEstimated)
	}
}

func TestUIShellServed(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("Trajectory console")) {
		t.Fatalf("missing shell markup: %s", rr.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodGet, "/ui/app.js", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("js status=%d", rr2.Code)
	}
}

func TestLogoAndFaviconServed(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")
	for _, path := range []string{"/ui/logo.png", "/ui/logo_transparent.png", "/favicon.ico"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, rr.Code)
		}
		if rr.Body.Len() < 100 {
			t.Fatalf("%s too small: %d", path, rr.Body.Len())
		}
		ct := rr.Header().Get("Content-Type")
		if !strings.Contains(ct, "image/png") && !strings.Contains(ct, "image/") {
			t.Fatalf("%s content-type %q", path, ct)
		}
	}
}

func TestHTTPHealthzNoAuth(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "secret")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHTTPRejectsPathTraversalTrajectory(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")
	e := sampleEvent(KindNodeAppended)
	e.TrajectoryID = "..\\evil"
	e.TS = time.Now().UTC().Format(time.RFC3339)
	body, _ := json.Marshal(e)
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHTTPSummaryRejectsBadFileAndBadID(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(st.Root(), "trajectories", "broken.ndjson")
	if err := os.WriteFile(path, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")

	for _, target := range []string{
		"/v1/trajectories/broken/summary",
		"/v1/trajectories/broken/events",
		"/v1/trajectories/..%2Fevil/summary",
		"/v1/trajectories/..%2Fevil/events",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", target, rr.Code, rr.Body.String())
		}
	}
}
