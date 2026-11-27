package tir

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

func limitLog(t *testing.T) *nodelog.NodeLog {
	t.Helper()
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	return nl
}

// Every node of a trajectory goes into nodes.ndjson, and Load rejects that
// member past MaxUncompressedEntryBytes. No single node here is near the
// limit, but together they pass it. Export must refuse rather than write a
// package verify cannot read.
func TestExportRefusesNodesOverEntryLimit(t *testing.T) {
	nl := limitLog(t)
	blob := strings.Repeat("a", 4<<20)
	for step := 1; step <= 9; step++ {
		s := step
		if _, err := nl.Append("PROJECT_CONTEXT", &s, map[string]any{"goal": "host-hook", "blob": blob}, "t-big", "local", 0); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "run.tir")
	tenant := "local"
	_, err := Export(nl, "t-big", dest, ExportOptions{TenantID: &tenant})
	if !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "nodes.ndjson") {
		t.Fatalf("err = %v, want a tir limit naming nodes.ndjson", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("export left files behind: %v", entries)
	}
}

// A trajectory under the limits still exports and loads.
func TestExportUnderLimitsStillLoads(t *testing.T) {
	nl := limitLog(t)
	blob := strings.Repeat("a", 4<<20)
	for step := 1; step <= 6; step++ {
		s := step
		if _, err := nl.Append("PROJECT_CONTEXT", &s, map[string]any{"goal": "host-hook", "blob": blob}, "t-ok", "local", 0); err != nil {
			t.Fatal(err)
		}
	}
	tenant := "local"
	path, err := Export(nl, "t-ok", filepath.Join(t.TempDir(), "run.tir"), ExportOptions{TenantID: &tenant})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}
