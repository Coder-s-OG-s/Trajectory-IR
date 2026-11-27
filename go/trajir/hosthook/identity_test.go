package hosthook_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestIdentityDefaults(t *testing.T) {
	proj := t.TempDir()
	id, err := hosthook.ResolveIdentity("claude", hosthook.Event{SessionID: "abc-123", Cwd: proj}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if id.WorkDir != filepath.Join(proj, ".trajir", "runs", "claude") {
		t.Fatalf("work dir = %s", id.WorkDir)
	}
	if id.TenantID != "local" || id.TrajectoryID != "claude-abc-123" || id.ProjectDir != proj {
		t.Fatalf("identity = %+v", id)
	}
}

func TestIdentityEnvOverrides(t *testing.T) {
	id, err := hosthook.ResolveIdentity("codex", hosthook.Event{SessionID: "s", Cwd: t.TempDir()}, env(map[string]string{
		"TRAJIR_WORK_DIR":      "/w",
		"TRAJIR_TENANT_ID":     "team",
		"TRAJIR_TRAJECTORY_ID": "fixed",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if id.WorkDir != "/w" || id.TenantID != "team" || id.TrajectoryID != "fixed" {
		t.Fatalf("identity = %+v", id)
	}
}

func TestIdentityHashesUnsafeSessionID(t *testing.T) {
	id, err := hosthook.ResolveIdentity("gemini", hosthook.Event{SessionID: `../x:y\z`, Cwd: t.TempDir()}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(id.TrajectoryID, `/\:.`) || !strings.HasPrefix(id.TrajectoryID, "gemini-s") {
		t.Fatalf("trajectory id = %q", id.TrajectoryID)
	}
}

func TestIdentityEmptySessionID(t *testing.T) {
	id, err := hosthook.ResolveIdentity("copilot", hosthook.Event{Cwd: t.TempDir()}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if id.TrajectoryID != "copilot-nosession" {
		t.Fatalf("trajectory id = %q", id.TrajectoryID)
	}
}

func TestPrepareWritesGitignore(t *testing.T) {
	proj := t.TempDir()
	id, err := hosthook.ResolveIdentity("claude", hosthook.Event{SessionID: "s", Cwd: proj}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := id.Prepare(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(proj, ".trajir", ".gitignore"))
	if err != nil || strings.TrimSpace(string(raw)) != "*" {
		t.Fatalf(".gitignore = %q, %v", raw, err)
	}
	if info, err := os.Stat(id.WorkDir); err != nil || !info.IsDir() {
		t.Fatalf("work dir not created: %v", err)
	}
}

func TestPrepareKeepsExistingGitignore(t *testing.T) {
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, ".trajir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".trajir", ".gitignore"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, _ := hosthook.ResolveIdentity("claude", hosthook.Event{SessionID: "s", Cwd: proj}, env(nil))
	if err := id.Prepare(); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(proj, ".trajir", ".gitignore"))
	if string(raw) != "mine\n" {
		t.Fatalf(".gitignore overwritten: %q", raw)
	}
}

func TestPrepareUnwritableNamesEnvVar(t *testing.T) {
	proj := t.TempDir()
	blocker := filepath.Join(proj, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	id, _ := hosthook.ResolveIdentity("claude", hosthook.Event{SessionID: "s", Cwd: proj},
		env(map[string]string{"TRAJIR_WORK_DIR": filepath.Join(blocker, "runs")}))
	err := id.Prepare()
	if err == nil || !strings.Contains(err.Error(), "TRAJIR_WORK_DIR") {
		t.Fatalf("want hint naming TRAJIR_WORK_DIR, got %v", err)
	}
}

func TestDefaultSinkIsUserFileSink(t *testing.T) {
	home := t.TempDir()
	sink, dir := hosthook.DefaultSink(env(map[string]string{"HOME": home, "USERPROFILE": home}))
	fs, ok := sink.(*emit.FileSink)
	if !ok || dir != filepath.Join(home, ".trajir", "console-data") || fs.Dir != dir {
		t.Fatalf("sink = %#v, dir = %s", sink, dir)
	}
}

func TestDefaultSinkDataEnv(t *testing.T) {
	sink, dir := hosthook.DefaultSink(env(map[string]string{"TRAJIR_CONSOLE_DATA": "/data"}))
	if fs, ok := sink.(*emit.FileSink); !ok || fs.Dir != "/data" || dir != "/data" {
		t.Fatalf("sink = %#v, dir = %s", sink, dir)
	}
}

func TestSinkOffSendsNothing(t *testing.T) {
	for _, v := range []string{"off", "OFF", "none", "0"} {
		if sink, _ := hosthook.DefaultSink(env(map[string]string{"TRAJIR_CONSOLE_SINK": v})); sink != nil {
			t.Fatalf("%s: sink = %#v", v, sink)
		}
	}
}

func TestEnvHTTPSinkWins(t *testing.T) {
	sink, _ := hosthook.DefaultSink(env(map[string]string{
		"TRAJIR_CONSOLE_SINK":  "http",
		"TRAJIR_CONSOLE_URL":   "http://127.0.0.1:8787",
		"TRAJIR_CONSOLE_TOKEN": "tok",
	}))
	h, ok := sink.(*emit.HTTPSink)
	if !ok || h.BaseURL != "http://127.0.0.1:8787" || h.Token != "tok" {
		t.Fatalf("sink = %#v", sink)
	}
}
