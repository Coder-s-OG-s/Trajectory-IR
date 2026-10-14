package console

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveEvidenceRootUsesCheckoutThatContainsTheDemo(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, ".console-data")
	demoDir := filepath.Join(root, "integrations", "langgraph")
	if err := os.MkdirAll(demoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(demoDir, "demo_stub.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRAJIR_EVIDENCE_ROOT", "")
	got, err := resolveEvidenceRoot(data)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(root) {
		t.Fatalf("got %s want %s", got, root)
	}
}

func TestResolveEvidenceRootDoesNotGuessAPrivateWorktree(t *testing.T) {
	parent := t.TempDir()
	consoleWT := filepath.Join(parent, "console-checkout")
	evidenceWT := filepath.Join(parent, "Trajectory-IR-wt-evidence")
	data := filepath.Join(consoleWT, ".console-data")
	demoDir := filepath.Join(evidenceWT, "integrations", "langgraph")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(demoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(demoDir, "demo_stub.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRAJIR_EVIDENCE_ROOT", "")
	if _, err := resolveEvidenceRoot(data); err == nil {
		t.Fatal("a private worktree name must not be a fallback")
	}
}

func testPython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		p, err := exec.LookPath(name)
		if err == nil {
			return p
		}
	}
	t.Skip("python not on PATH")
	return ""
}

func TestRunLangGraphDemoStopsAndSendsToken(t *testing.T) {
	root := t.TempDir()
	demoDir := filepath.Join(root, "integrations", "langgraph")
	if err := os.MkdirAll(demoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := []byte("import os, sys, time\nsys.stdout.write(os.environ.get('TRAJIR_CONSOLE_TOKEN', '') + '\\n')\nsys.stdout.flush()\ntime.sleep(30)\n")
	if err := os.WriteFile(filepath.Join(demoDir, "demo_stub.py"), script, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(st, "demo-token")
	srv.DemoTimeout = 3 * time.Second
	t.Setenv("TRAJIR_EVIDENCE_ROOT", root)
	t.Setenv("TRAJIR_PYTHON", testPython(t))
	req := httptest.NewRequest(http.MethodPost, "/v1/local/run-langgraph-demo", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Authorization", "Bearer demo-token")
	rec := httptest.NewRecorder()
	start := time.Now()
	srv.Handler().ServeHTTP(rec, req)
	if time.Since(start) > 12*time.Second {
		t.Fatal("demo was not stopped")
	}
	body := rec.Body.String()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d body %s", rec.Code, body)
	}
	if !strings.Contains(body, "demo-token") {
		t.Fatalf("token was not passed to the demo: %s", body)
	}
}

func TestResolveEvidenceRootEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TRAJIR_EVIDENCE_ROOT", dir)
	got, err := resolveEvidenceRoot(filepath.Join(t.TempDir(), ".console-data"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(dir) {
		t.Fatalf("got %s want %s", got, dir)
	}
}
