package audit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/audit"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

func exportGood(t *testing.T, dir string) string {
	t.Helper()
	nl, err := nodelog.Open(filepath.Join(dir, "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer nl.Close()
	step := 1
	if _, err := nl.Append("PROJECT_CONTEXT", &step, map[string]any{"goal": "ship"}, "t-audit", "demo", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("DECISION", &step, map[string]any{
		"plan": map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "echo", "args": map[string]any{"msg": "hi"}},
			},
		},
	}, "t-audit", "demo", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{
		"tool": "echo",
		"args": map[string]any{"msg": "hi"},
	}, "t-audit", "demo", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{"result": "hi"}, "t-audit", "demo", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("COMMIT_STEP", &step, map[string]any{}, "t-audit", "demo", 4); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "good.tir")
	tenant := "demo"
	if _, err := tir.Export(nl, "t-audit", out, tir.ExportOptions{Mode: tir.ModeThin, TenantID: &tenant}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestVerifyFileGoodPack(t *testing.T) {
	dir := t.TempDir()
	path := exportGood(t, dir)
	res, err := audit.VerifyFile(path, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("findings=%v", res.Findings)
	}
}

func TestVerifyFileBrokenHash(t *testing.T) {
	dir := t.TempDir()
	path := exportGood(t, dir)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the middle of the zip so Load fails verification.
	if len(raw) < 40 {
		t.Fatalf("pack too small: %d", len(raw))
	}
	raw[len(raw)/2] ^= 0xff
	bad := filepath.Join(dir, "bad.tir")
	if err := os.WriteFile(bad, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := audit.VerifyFile(bad, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected load failure")
	}
	if len(res.Findings) == 0 || res.Findings[0].Code != "LOAD_FAILED" {
		t.Fatalf("findings=%v", res.Findings)
	}
}

func TestVerifySealMissingBeforeTool(t *testing.T) {
	dir := t.TempDir()
	nl, err := nodelog.Open(filepath.Join(dir, "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer nl.Close()
	step := 1
	if _, err := nl.Append("PROJECT_CONTEXT", &step, map[string]any{"goal": "x"}, "t-noseal", "demo", 0); err != nil {
		t.Fatal(err)
	}
	// TOOL_CALL without DECISION
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{
		"tool": "deploy_server",
		"args": map[string]any{"version": "1.0.0"},
	}, "t-noseal", "demo", 2); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "noseal.tir")
	tenant := "demo"
	if _, err := tir.Export(nl, "t-noseal", out, tir.ExportOptions{Mode: tir.ModeThin, TenantID: &tenant}); err != nil {
		t.Fatal(err)
	}
	res, err := audit.VerifyFile(out, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected SEAL_BEFORE_EXECUTE")
	}
	found := false
	for _, f := range res.Findings {
		if f.Code == "SEAL_BEFORE_EXECUTE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings=%v", res.Findings)
	}
}

func TestVerifyOpenWorldLieOnToolCall(t *testing.T) {
	dir := t.TempDir()
	nl, err := nodelog.Open(filepath.Join(dir, "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer nl.Close()
	step := 1
	if _, err := nl.Append("PROJECT_CONTEXT", &step, map[string]any{}, "t-lie", "demo", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("DECISION", &step, map[string]any{
		"plan": map[string]any{"tool_calls": []any{
			map[string]any{"name": "bash", "args": map[string]any{"cmd": "rm -rf /"}, "effect_class": "READ_ONLY"},
		}},
	}, "t-lie", "demo", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{
		"tool":         "bash",
		"args":         map[string]any{"cmd": "rm -rf /"},
		"effect_class": "READ_ONLY",
	}, "t-lie", "demo", 2); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "lie.tir")
	tenant := "demo"
	if _, err := tir.Export(nl, "t-lie", out, tir.ExportOptions{Mode: tir.ModeThin, TenantID: &tenant}); err != nil {
		t.Fatal(err)
	}
	res, err := audit.VerifyFile(out, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("expected OPEN_WORLD_LIE")
	}
	joined := ""
	for _, f := range res.Findings {
		joined += f.Code + " " + f.Message + "\n"
	}
	if !strings.Contains(joined, "OPEN_WORLD_LIE") {
		t.Fatalf("findings=%v", res.Findings)
	}
}

func TestVerifyRequireSignature(t *testing.T) {
	dir := t.TempDir()
	path := exportGood(t, dir)
	res, err := audit.VerifyFile(path, audit.Options{RequireSignature: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Fatal("unsigned pack should fail with RequireSignature")
	}
	found := false
	for _, f := range res.Findings {
		if f.Code == "SIGNATURE_REQUIRED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("findings=%v", res.Findings)
	}
}
