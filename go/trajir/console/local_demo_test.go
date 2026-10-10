package console

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEvidenceRootSibling(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	consoleWT := filepath.Join(parent, "Trajectory-IR-wt-console")
	evidenceWT := filepath.Join(parent, "Trajectory-IR-wt-evidence")
	data := filepath.Join(consoleWT, ".console-data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(evidenceWT, "integrations", "langgraph"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveEvidenceRoot(data)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(evidenceWT) {
		t.Fatalf("got %s want %s", got, evidenceWT)
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
