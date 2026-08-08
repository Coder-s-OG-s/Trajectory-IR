package graft_test

import (
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/graft"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

func TestErrorText(t *testing.T) {
	e := &graft.Error{Msg: "boom"}
	if e.Error() != "boom" {
		t.Fatalf("Error()=%q", e.Error())
	}
}

func TestFindArtifactRefEmptyContentHash(t *testing.T) {
	_, err := graft.FindArtifactRef(nil, "")
	if err == nil {
		t.Fatal("expected error for empty content_hash")
	}
}

func TestFindArtifactRefNoMatch(t *testing.T) {
	_, err := graft.FindArtifactRef([]map[string]any{
		{"kind": "ARTIFACT_PUT", "payload": map[string]any{"content_hash": "other"}},
	}, "missing")
	if err == nil {
		t.Fatal("expected error when no artifact ref matches")
	}
}

func TestFindArtifactRefSkipsPrivateAndUnrelatedKinds(t *testing.T) {
	h := "ab" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	nodes := []map[string]any{
		{"kind": "THOUGHT", "payload": map[string]any{"content_hash": h}},
		{"kind": "DECISION", "payload": map[string]any{"content_hash": h}},
		{"kind": "ARTIFACT_PUT", "payload": map[string]any{"content_hash": h}, "seq": 1},
	}
	got, err := graft.FindArtifactRef(nodes, h)
	if err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "ARTIFACT_PUT" {
		t.Fatalf("kind=%v", got["kind"])
	}
}

func TestFindArtifactRefPrefersRefOverPutAndLowerSeq(t *testing.T) {
	h := "cd" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	nodes := []map[string]any{
		{"kind": "ARTIFACT_PUT", "payload": map[string]any{"content_hash": h}, "seq": 0},
		{"kind": "ARTIFACT_REF", "payload": map[string]any{"content_hash": h}, "seq": 5},
		{"kind": "ARTIFACT_REF", "payload": map[string]any{"content_hash": h}, "seq": 2},
	}
	got, err := graft.FindArtifactRef(nodes, h)
	if err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "ARTIFACT_REF" || got["seq"] != 2 {
		t.Fatalf("got=%v", got)
	}
}

func TestGraftArtifactRefNilTarget(t *testing.T) {
	_, err := graft.GraftArtifactRef(nil, graft.GraftOptions{ContentHash: "x"})
	if err == nil {
		t.Fatal("expected error for nil target")
	}
}

func TestGraftArtifactRefExplicitOverridesWinOverSource(t *testing.T) {
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
	h := "ef" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	if _, err := src.Append("ARTIFACT_PUT", &step, map[string]any{
		"content_hash": h,
		"logical_path": "src/out.bin",
		"uri":          "file:///src/out.bin",
	}, "src", "demo", 0); err != nil {
		t.Fatal(err)
	}
	nodes, err := src.ListNodes("src", "demo")
	if err != nil {
		t.Fatal(err)
	}
	n, err := graft.GraftArtifactRef(dst, graft.GraftOptions{
		ContentHash:         h,
		TargetTrajectoryID:  "dst",
		TargetTenantID:      "demo",
		Seq:                 0,
		StepN:               &step,
		SourceNodes:         nodes,
		LogicalPath:         "override.bin",
		HasLogicalPath:      true,
		SourceTrajectoryID:  "explicit-src",
		HasSourceTrajectory: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.Payload["logical_path"] != "override.bin" {
		t.Fatalf("logical_path=%v", n.Payload["logical_path"])
	}
	if n.Payload["source_trajectory_id"] != "explicit-src" {
		t.Fatalf("source_trajectory_id=%v", n.Payload["source_trajectory_id"])
	}
	if n.Payload["uri"] != "file:///src/out.bin" {
		t.Fatalf("uri=%v", n.Payload["uri"])
	}
}

func TestGraftArtifactRefWithoutSourceNodes(t *testing.T) {
	dst, err := nodelog.Open(filepath.Join(t.TempDir(), "dst.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	step := 1
	h := "11" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	n, err := graft.GraftArtifactRef(dst, graft.GraftOptions{
		ContentHash:        h,
		TargetTrajectoryID: "dst",
		TargetTenantID:     "demo",
		Seq:                0,
		StepN:              &step,
		LogicalPath:        "manual.bin",
		HasLogicalPath:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.Payload["content_hash"] != h || n.Payload["logical_path"] != "manual.bin" {
		t.Fatalf("payload=%v", n.Payload)
	}
}

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
