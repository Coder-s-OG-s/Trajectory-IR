package conformance_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func buildCrashAgent(t *testing.T) (string, bool) {
	t.Helper()
	root := moduleRoot(t)
	binDir := filepath.Join(root, ".bin")
	_ = os.MkdirAll(binDir, 0o755)
	out := filepath.Join(binDir, "crashagent")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "./cmd/crashagent")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build crashagent: %v\n%s", err, outBytes)
	}
	// Probe whether this host allows running the binary (Windows Application Control may block).
	probe := exec.Command(out, "-h")
	if err := probe.Run(); err != nil {
		// -h is not defined; any start failure is the signal we care about.
		if _, ok := err.(*exec.ExitError); ok {
			return out, true
		}
		if strings.Contains(err.Error(), "Application Control") || strings.Contains(err.Error(), "blocked") {
			return out, false
		}
		// Try a real short start: if fork/exec fails, skip subprocess tests.
		return out, false
	}
	return out, true
}

func canExecAgent(t *testing.T, bin string) bool {
	t.Helper()
	cmd := exec.Command(bin, "-workdir", t.TempDir())
	// Will fail quickly if flag path missing files mid-run is fine; we only care about fork/exec.
	err := cmd.Start()
	if err != nil {
		if strings.Contains(err.Error(), "Application Control") || strings.Contains(err.Error(), "blocked") || strings.Contains(err.Error(), "fork/exec") {
			return false
		}
		return false
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	return true
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func readCounter(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func TestR01NoReinferAfterDecisionSeal_Subprocess(t *testing.T) {
	bin, _ := buildCrashAgent(t)
	if !canExecAgent(t, bin) {
		t.Skip("subprocess crashagent blocked on this host (e.g. Windows Application Control); see in-process tests")
	}
	workdir := t.TempDir()
	marker := filepath.Join(workdir, "decision_sealed.marker")

	crash := exec.Command(bin, "-workdir", workdir, "-crash-after=decision_sealed")
	if err := crash.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, marker, 15*time.Second)
	if err := crash.Process.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	_, _ = crash.Process.Wait()

	resumeCmd := exec.Command(bin, "-workdir", workdir, "-resume")
	out, err := resumeCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resume: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "step committed") {
		t.Fatalf("resume output missing commit:\n%s", out)
	}
	if n := readCounter(filepath.Join(workdir, "test_model_call_count.txt")); n != 1 {
		t.Fatalf("model calls=%d, want 1", n)
	}
	if n := readCounter(filepath.Join(workdir, "test_deploy_side_effect_count.txt")); n != 1 {
		t.Fatalf("deploy count=%d, want 1", n)
	}
}

func TestR02BlockWhenToolCallInterrupted_Subprocess(t *testing.T) {
	bin, _ := buildCrashAgent(t)
	if !canExecAgent(t, bin) {
		t.Skip("subprocess crashagent blocked on this host; see in-process tests")
	}
	workdir := t.TempDir()
	marker := filepath.Join(workdir, "tool_started.marker")

	crash := exec.Command(bin, "-workdir", workdir, "-crash-during=tool_call")
	if err := crash.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, marker, 15*time.Second)
	if err := crash.Process.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	_, _ = crash.Process.Wait()

	if n := readCounter(filepath.Join(workdir, "test_deploy_side_effect_count.txt")); n != 0 {
		t.Fatalf("deploy count after kill=%d, want 0", n)
	}

	resumeCmd := exec.Command(bin, "-workdir", workdir, "-resume")
	out, err := resumeCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resume: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "BLOCKED_NEEDS_GATE") {
		t.Fatalf("resume missing block:\n%s", out)
	}
	if n := readCounter(filepath.Join(workdir, "test_deploy_side_effect_count.txt")); n != 0 {
		t.Fatalf("deploy count after resume=%d, want 0", n)
	}
	if n := readCounter(filepath.Join(workdir, "test_model_call_count.txt")); n != 1 {
		t.Fatalf("model calls=%d, want 1", n)
	}
}

// In-process R01: crash inject via panic after DECISION, then resume same DBs.
func TestR01NoReinferAfterDecisionSeal_InProcess(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nodes.sqlite")
	memoPath := filepath.Join(dir, "memo.sqlite")

	var modelCalls atomic.Int32
	var deploys atomic.Int32

	runOnce := func(crashAfterSeal bool) error {
		nl, err := nodelog.Open(logPath)
		if err != nil {
			return err
		}
		defer nl.Close()
		backend, err := durable.OpenLocal(memoPath)
		if err != nil {
			return err
		}
		defer backend.Close()

		cfg := resume.RunStepConfig{
			Log:          nl,
			Backend:      backend,
			TenantID:     "demo",
			TrajectoryID: "t1",
			WorkflowID:   "wf-r01",
			Tools: map[string]resume.Tool{
				"deploy_server": {
					Name:   "deploy_server",
					Effect: effects.NON_IDEMPOTENT_WRITE,
					Fn: func(map[string]any) (any, error) {
						deploys.Add(1)
						return "ok", nil
					},
				},
			},
			OnDecisionSealed: func() {
				if crashAfterSeal {
					panic("inject crash after decision seal")
				}
			},
		}
		model := func(context.Context, map[string]any) (map[string]any, error) {
			modelCalls.Add(1)
			return map[string]any{
				"tool_calls": []any{
					map[string]any{"name": "deploy_server", "args": map[string]any{}},
				},
			}, nil
		}
		_, err = resume.RunStep(ctx, cfg, 1, model, map[string]any{})
		return err
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic after seal")
			}
		}()
		_ = runOnce(true)
	}()

	if err := runOnce(false); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if modelCalls.Load() != 1 {
		t.Fatalf("model calls=%d, want 1", modelCalls.Load())
	}
	if deploys.Load() != 1 {
		t.Fatalf("deploys=%d, want 1", deploys.Load())
	}
}

// In-process R02: TOOL_CALL already logged (crash after call start), resume blocks.
func TestR02BlockWhenToolCallInterrupted_InProcess(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	nl, err := nodelog.Open(filepath.Join(dir, "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer nl.Close()
	backend, err := durable.OpenLocal(filepath.Join(dir, "memo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	step, seq := 1, 2
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{"tool": "deploy_server", "args": map[string]any{}}, "t1", "demo", seq); err != nil {
		t.Fatal(err)
	}

	var deploys atomic.Int32
	var modelCalls atomic.Int32
	cfg := resume.RunStepConfig{
		Log:          nl,
		Backend:      backend,
		TenantID:     "demo",
		TrajectoryID: "t1",
		WorkflowID:   "wf-r02",
		Tools: map[string]resume.Tool{
			"deploy_server": {
				Name:   "deploy_server",
				Effect: effects.NON_IDEMPOTENT_WRITE,
				Fn: func(map[string]any) (any, error) {
					deploys.Add(1)
					return "ok", nil
				},
			},
		},
	}
	model := func(context.Context, map[string]any) (map[string]any, error) {
		modelCalls.Add(1)
		return map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "deploy_server", "args": map[string]any{}},
			},
		}, nil
	}

	_, err = resume.RunStep(ctx, cfg, 1, model, map[string]any{})
	var blocked *resume.BlockedNeedsGate
	if !errors.As(err, &blocked) {
		t.Fatalf("want BlockedNeedsGate, got %v", err)
	}
	if deploys.Load() != 0 {
		t.Fatalf("deploys=%d, want 0", deploys.Load())
	}
	if modelCalls.Load() != 1 {
		t.Fatalf("model calls=%d, want 1", modelCalls.Load())
	}
}
