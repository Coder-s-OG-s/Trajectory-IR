package graft_test

import (
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/graft"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

func TestGraftArtifactNotThought(t *testing.T) {
	src, err := nodelog.Open(filepath.Join(t.TempDir(), "src.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })
	dst, err := nodelog.Open(filepath.Join(t.TempDir(), "dst.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	step := 1
	h := "ab" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	if _, err := src.Append("THOUGHT", &step, map[string]any{"text": "private"}, "src", "demo", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Append("ARTIFACT_PUT", &step, map[string]any{
		"content_hash": h,
		"logical_path": "out.bin",
	}, "src", "demo", 1); err != nil {
		t.Fatal(err)
	}
	nodes, err := src.ListNodes("src", "demo")
	if err != nil {
		t.Fatal(err)
	}
	n, err := graft.GraftArtifactRef(dst, graft.GraftOptions{
		ContentHash:        h,
		TargetTrajectoryID: "dst",
		TargetTenantID:     "demo",
		Seq:                0,
		StepN:              &step,
		SourceNodes:        nodes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.Kind != "ARTIFACT_REF" {
		t.Fatalf("kind=%s", n.Kind)
	}
	out, err := dst.ListNodes("dst", "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range out {
		if row["kind"] == "THOUGHT" {
			t.Fatal("THOUGHT leaked into target")
		}
	}
}
