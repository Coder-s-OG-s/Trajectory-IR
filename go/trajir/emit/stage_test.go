package emit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

func TestStagePackageCopiesTir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "run.tir")
	if err := os.WriteFile(src, []byte("PK-demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := emit.StagePackage(dir, "t1", src, "evt1")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "packages/t1/run.tir" {
		t.Fatalf("rel=%s", rel)
	}
	got, err := os.ReadFile(filepath.Join(dir, "packages", "t1", "run.tir"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "PK-demo" {
		t.Fatalf("copied %q", got)
	}
	rel2, err := emit.StagePackage(dir, "t1", src, "evt1")
	if err != nil || rel2 != rel {
		t.Fatalf("idempotent copy rel=%s err=%v", rel2, err)
	}
}

func TestStagePackageRejectsBadIDAndMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := emit.StagePackage(dir, "../x", "no.tir", ""); err == nil {
		t.Fatal("expected bad id")
	}
	if _, err := emit.StagePackage(dir, "t1", filepath.Join(dir, "missing.tir"), ""); err == nil {
		t.Fatal("expected missing")
	}
}

func TestSafeTirName(t *testing.T) {
	if _, err := emit.SafeTirName("ok.tir"); err != nil {
		t.Fatal(err)
	}
	if _, err := emit.SafeTirName("../x.tir"); err == nil {
		t.Fatal("expected reject")
	}
	if _, err := emit.SafeTirName("nope.zip"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestFileSinkStagesExport(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "out.tir")
	if err := os.WriteFile(src, []byte("tir-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	sink := &emit.FileSink{Dir: dir}
	if err := sink.Emit(emit.Event{
		Kind:         emit.KindExportCompleted,
		TrajectoryID: "pack",
		Payload: map[string]any{
			"path": src, "mode": "thin", "redacted": true,
			"bytes": 9, "member_count": 5, "node_count": 1, "ok": true,
		},
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "trajectories", "pack.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "packages/pack/out.tir") {
		t.Fatalf("missing console_path: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "packages", "pack", "out.tir")); err != nil {
		t.Fatal(err)
	}
}
