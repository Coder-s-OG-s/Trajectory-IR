package console

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreStagesExportAndLists(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "demo.tir")
	if err := os.WriteFile(src, append([]byte{0x50, 0x4b, 0x03, 0x04}, "package-bytes"...), 0o644); err != nil {
		t.Fatal(err)
	}
	e := sampleEvent(KindExportCompleted)
	e.Payload = json.RawMessage(`{"path":` + jsonString(src) + `,"mode":"thin","redacted":false,"bytes":17,"member_count":5,"node_count":1,"ok":true}`)
	if err := st.Append(e); err != nil {
		t.Fatal(err)
	}
	items, err := st.ListPackages(e.TrajectoryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "demo.tir" || items[0].Bytes != 17 {
		t.Fatalf("items=%+v", items)
	}
	events, err := st.ReadEvents(e.TrajectoryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d", len(events))
	}
	var payload map[string]any
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["console_path"] != "packages/t-demo/demo.tir" {
		t.Fatalf("payload=%v", payload)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestHTTPPackagesAndLoopback(t *testing.T) {
	root := t.TempDir()
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(root, "packages", "t-demo")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "demo.tir"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")

	req := httptest.NewRequest(http.MethodGet, "/v1/trajectories/t-demo/packages", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/trajectories/t-demo/packages/demo.tir", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK || rr2.Body.String() != "abc" {
		t.Fatalf("get status=%d body=%q", rr2.Code, rr2.Body.String())
	}

	req3 := httptest.NewRequest(http.MethodGet, "/v1/trajectories/t-demo/packages/../secret.tir", nil)
	rr3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr3, req3)
	if rr3.Code == http.StatusOK {
		t.Fatalf("traversal succeeded: %d %s", rr3.Code, rr3.Body.String())
	}

	orig := startLocal
	defer func() { startLocal = orig }()
	var started []string
	startLocal = func(name string, args ...string) error {
		started = append(started, name)
		return nil
	}

	req4 := httptest.NewRequest(http.MethodPost, "/v1/local/reveal", nil)
	req4.RemoteAddr = "8.8.8.8:9"
	rr4 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status=%d", rr4.Code)
	}

	req5 := httptest.NewRequest(http.MethodPost, "/v1/local/reveal", nil)
	req5.RemoteAddr = "127.0.0.1:9"
	req5.Host = "127.0.0.1:8787"
	req5.Header.Set("Origin", "http://127.0.0.1:8787")
	rr5 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusOK {
		t.Fatalf("reveal status=%d body=%s", rr5.Code, rr5.Body.String())
	}
	if len(started) != 1 {
		t.Fatalf("started=%v", started)
	}

	req6 := httptest.NewRequest(http.MethodPost, "/v1/local/open-shell", nil)
	req6.RemoteAddr = "127.0.0.1:9"
	req6.Host = "127.0.0.1:8787"
	req6.Header.Set("Origin", "http://127.0.0.1:8787")
	rr6 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusOK {
		t.Fatalf("shell status=%d body=%s", rr6.Code, rr6.Body.String())
	}

	// A page on another site can reach 127.0.0.1 too. It must not start a process.
	started = nil
	crossSite := []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		func(r *http.Request) { r.Host = "rebind.evil.example:8787" },
	}
	for i, mutate := range crossSite {
		for _, path := range []string{"/v1/local/reveal", "/v1/local/open-shell"} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"root":true}`))
			req.RemoteAddr = "127.0.0.1:9"
			req.Host = "127.0.0.1:8787"
			mutate(req)
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("case %d %s status=%d", i, path, rr.Code)
			}
		}
	}
	if len(started) != 0 {
		t.Fatalf("cross-site started=%v", started)
	}
}

func TestStoreDoesNotStageNonTir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(secret, []byte("TOP-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := sampleEvent(KindExportCompleted)
	e.Payload = json.RawMessage(`{"path":` + jsonString(secret) + `,"mode":"thin","redacted":false,"bytes":10,"member_count":5,"node_count":1,"ok":true}`)
	if err := st.Append(e); err != nil {
		t.Fatal(err)
	}
	items, err := st.ListPackages(e.TrajectoryID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("staged a non-.tir file: %+v", items)
	}
}

func TestRevealArgsWindowsSelectsFile(t *testing.T) {
	name, args := revealArgs(`C:\data\packages\t\demo.tir`)
	if name == "" || len(args) == 0 {
		t.Fatal("empty reveal args")
	}
	name2, args2 := openShellArgs(`C:\data`)
	if name2 == "" || len(args2) == 0 {
		t.Fatal("empty shell args")
	}
}

func TestRevealArgsOpensDirectoryNotParentSelect(t *testing.T) {
	dir := t.TempDir()
	_, args := revealArgs(dir)
	if len(args) == 0 {
		t.Fatal("empty reveal args")
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "/select,") {
		t.Fatalf("directory must open itself, not /select, parent: %v", args)
	}
	if args[len(args)-1] != dir && args[0] != dir {
		t.Fatalf("directory path missing from args=%v", args)
	}
}

func TestUIHasLocalPackageControls(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	body := rr.Body.String()
	for _, want := range []string{"Show data folder", "Open PowerShell here"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in html", want)
		}
	}
	req2 := httptest.NewRequest(http.MethodGet, "/ui/app.js", nil)
	rr2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr2, req2)
	js := rr2.Body.String()
	for _, want := range []string{"Show in folder", "/v1/local/reveal", "downloadPackage"} {
		if !strings.Contains(js, want) {
			t.Fatalf("missing %q in js", want)
		}
	}
}
