package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
)

func TestLiveHostStep(t *testing.T) {
	dir := t.TempDir()
	step, err := runHostStep(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(step.ToolResults) != 2 {
		t.Fatalf("tool results=%d want 2", len(step.ToolResults))
	}
	kinds := map[string]bool{}
	for _, k := range step.NodeKinds {
		kinds[k] = true
	}
	for _, need := range []string{"PROJECT_CONTEXT", "DECISION", "COMMIT_STEP"} {
		if !kinds[need] {
			t.Fatalf("missing kind %s in %v", need, step.NodeKinds)
		}
	}
}

func TestSandboxRejectsShip(t *testing.T) {
	dir := t.TempDir()
	_, err := runHostStep(dir, true, nil)
	var forbidden *sandbox.Forbidden
	if !errors.As(err, &forbidden) {
		t.Fatalf("err=%v want sandbox.Forbidden", err)
	}
}

func TestCustomModelPureOnly(t *testing.T) {
	dir := t.TempDir()
	model := func(ctx map[string]any) map[string]any {
		return map[string]any{
			"tool_calls": []any{
				map[string]any{
					"name": "build_manifest",
					"args": map[string]any{
						"service": ctx["service"],
						"release": ctx["release"],
					},
				},
			},
		}
	}
	step, err := runHostStep(dir, false, model)
	if err != nil {
		t.Fatal(err)
	}
	if len(step.ToolResults) != 1 {
		t.Fatalf("results=%d", len(step.ToolResults))
	}
}

func TestExportThinPackage(t *testing.T) {
	dir := t.TempDir()
	step, err := runHostStep(dir, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := reportBytes(step)
	if err != nil {
		t.Fatal(err)
	}
	casRoot := filepath.Join(dir, "cas")
	dest := filepath.Join(dir, "run.tir")
	pkg, err := exportThinPackage(dir, casRoot, dest, payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pkg.TirPath); err != nil {
		t.Fatal(err)
	}
	if string(pkg.Rehydrated) != string(payload) {
		t.Fatalf("rehydrate mismatch")
	}
	if pkg.NodeCount < 1 {
		t.Fatalf("node_count=%d", pkg.NodeCount)
	}
}

func TestExportRejectsEmptyPayload(t *testing.T) {
	dir := t.TempDir()
	if _, err := runHostStep(dir, false, nil); err != nil {
		t.Fatal(err)
	}
	_, err := exportThinPackage(dir, filepath.Join(dir, "cas"), filepath.Join(dir, "x.tir"), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
