package tir_test

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

func openLog(t *testing.T, name string) *nodelog.NodeLog {
	t.Helper()
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	return nl
}

func seedSample(t *testing.T, nl *nodelog.NodeLog) {
	t.Helper()
	step := 1
	traj, tenant := "t-export", "demo"
	if _, err := nl.Append("PROJECT_CONTEXT", &step, map[string]any{"goal": "demo"}, traj, tenant, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("DECISION", &step, map[string]any{
		"plan": map[string]any{
			"tool_calls": []any{
				map[string]any{"name": "echo", "args": map[string]any{"msg": "hi"}},
			},
		},
	}, traj, tenant, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("TOOL_CALL", &step, map[string]any{
		"tool": "echo", "args": map[string]any{"msg": "hi"},
	}, traj, tenant, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("TOOL_RESULT", &step, map[string]any{"result": "hi"}, traj, tenant, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := nl.Append("COMMIT_STEP", &step, map[string]any{}, traj, tenant, 4); err != nil {
		t.Fatal(err)
	}
}

func TestExportImportRoundTripThin(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)

	out := filepath.Join(t.TempDir(), "run.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	dest := openLog(t, "dest.sqlite")
	pkg, err := tir.Import(path, dest)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest["mode"] != "thin" {
		t.Fatalf("mode=%v", pkg.Manifest["mode"])
	}
	if pkg.Manifest["trajectory_id"] != "t-export" {
		t.Fatalf("trajectory_id=%v", pkg.Manifest["trajectory_id"])
	}
	if len(pkg.Nodes) != 5 {
		t.Fatalf("nodes=%d", len(pkg.Nodes))
	}
	id, _ := pkg.Nodes[0]["id"].(string)
	n, err := dest.Count(id)
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}

	srcNodes, err := src.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	dstNodes, err := dest.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(dstNodes) != len(srcNodes) {
		t.Fatalf("len dst=%d src=%d", len(dstNodes), len(srcNodes))
	}
	for i := range srcNodes {
		if srcNodes[i]["id"] != dstNodes[i]["id"] {
			t.Fatalf("id[%d] %v vs %v", i, srcNodes[i]["id"], dstNodes[i]["id"])
		}
	}
}

func TestExportImportRoundTripFat(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)

	blob := []byte("print('hello')\n")
	sum := sha256.Sum256(blob)
	h := hex.EncodeToString(sum[:])

	out := filepath.Join(t.TempDir(), "fat.tir")
	_, err := tir.Export(src, "t-export", out, tir.ExportOptions{
		Mode: tir.ModeFat,
		Artifacts: []tir.ArtifactRef{
			{LogicalPath: "src/main.py", ContentHash: h},
		},
		ArtifactBytes: map[string][]byte{h: blob},
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := tir.Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest["mode"] != "fat" {
		t.Fatalf("mode=%v", pkg.Manifest["mode"])
	}
	got, ok := pkg.ArtifactBytes[h]
	if !ok || string(got) != string(blob) {
		t.Fatalf("artifact missing or mismatch")
	}
}

func TestImportDetectsTamperedNode(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	good := filepath.Join(t.TempDir(), "good.tir")
	if _, err := tir.Export(src, "t-export", good, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(t.TempDir(), "bad.tir")
	if err := rewriteNodes(good, bad, func(lines [][]byte) [][]byte {
		var rec map[string]any
		if err := json.Unmarshal(lines[1], &rec); err != nil {
			t.Fatal(err)
		}
		rec["payload"] = map[string]any{"plan": map[string]any{"tool_calls": []any{
			map[string]any{"name": "evil"},
		}}}
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		lines[1] = b
		return lines
	}); err != nil {
		t.Fatal(err)
	}

	_, err := tir.Load(bad)
	if err == nil {
		t.Fatal("expected verification error")
	}
	if !errors.Is(err, tir.ErrVerification) {
		t.Fatalf("err=%v want ErrVerification", err)
	}
}

func TestIdempotentReimport(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "run.tir")
	if _, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	dest := openLog(t, "dest.sqlite")
	if _, err := tir.Import(out, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := tir.Import(out, dest); err != nil {
		t.Fatal(err)
	}
	nodes, err := dest.ListNodes("t-export", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 5 {
		t.Fatalf("len=%d", len(nodes))
	}
}

func TestThinRejectsEmbeddedBytes(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	good := filepath.Join(t.TempDir(), "good.tir")
	if _, err := tir.Export(src, "t-export", good, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	// 64 hex chars that will not match content hash of "nope"
	h := "ab" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	bad := filepath.Join(t.TempDir(), "thin_with_bytes.tir")
	if err := injectZipMember(good, bad, "artifacts/cas/ab/"+h, []byte("nope")); err != nil {
		t.Fatal(err)
	}
	_, err := tir.Load(bad)
	if err == nil {
		t.Fatal("expected error for thin+embedded")
	}
}

func TestRejectsUnexpectedArtifactPath(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	good := filepath.Join(t.TempDir(), "good.tir")
	if _, err := tir.Export(src, "t-export", good, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "rogue_artifact_path.tir")
	if err := injectZipMember(good, bad, "artifacts/rogue/x", []byte("x")); err != nil {
		t.Fatal(err)
	}
	_, err := tir.Load(bad)
	if !errors.Is(err, tir.ErrVerification) {
		t.Fatalf("err=%v want ErrVerification", err)
	}
}

func TestRejectsPathTraversalMember(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	good := filepath.Join(t.TempDir(), "good.tir")
	if _, err := tir.Export(src, "t-export", good, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "traversal.tir")
	if err := injectZipMember(good, bad, "../evil.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	_, err := tir.Load(bad)
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestLoadUnverifiedAPIExists(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "run.tir")
	if _, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	pkg, err := tir.LoadUnverified(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Nodes) != 5 {
		t.Fatalf("nodes=%d", len(pkg.Nodes))
	}
}

func TestExportRejectsNilNodeLog(t *testing.T) {
	_, err := tir.Export(nil, "t1", filepath.Join(t.TempDir(), "x.tir"), tir.ExportOptions{Mode: tir.ModeThin})
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestExportRejectsUnsupportedMode(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	_, err := tir.Export(src, "t-export", filepath.Join(t.TempDir(), "x.tir"), tir.ExportOptions{Mode: "bogus"})
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestExportRejectsUnknownTrajectory(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	_, err := tir.Export(src, "does-not-exist", filepath.Join(t.TempDir(), "x.tir"), tir.ExportOptions{Mode: tir.ModeThin})
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestExportRejectsMixedTenants(t *testing.T) {
	src := openLog(t, "src.sqlite")
	step := 1
	if _, err := src.Append("PROJECT_CONTEXT", &step, map[string]any{"goal": "a"}, "t-mixed", "tenant-a", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Append("PROJECT_CONTEXT", &step, map[string]any{"goal": "b"}, "t-mixed", "tenant-b", 1); err != nil {
		t.Fatal(err)
	}
	_, err := tir.Export(src, "t-mixed", filepath.Join(t.TempDir(), "x.tir"), tir.ExportOptions{Mode: tir.ModeThin})
	if err == nil {
		t.Fatal("expected mixed tenant error")
	}
}

func TestExportFatRejectsMissingArtifactBytes(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	h := "22" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	_, err := tir.Export(src, "t-export", filepath.Join(t.TempDir(), "x.tir"), tir.ExportOptions{
		Mode:      tir.ModeFat,
		Artifacts: []tir.ArtifactRef{{LogicalPath: "a.bin", ContentHash: h}},
	})
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestExportFatRejectsContentHashMismatch(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	h := "33" + "00000000000000000000000000000000000000000000000000000000000000"[:62]
	_, err := tir.Export(src, "t-export", filepath.Join(t.TempDir(), "x.tir"), tir.ExportOptions{
		Mode:          tir.ModeFat,
		Artifacts:     []tir.ArtifactRef{{LogicalPath: "a.bin", ContentHash: h}},
		ArtifactBytes: map[string][]byte{h: []byte("does not match hash")},
	})
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestExportFatRejectsOversizedArtifact(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	big := make([]byte, 32*1024*1024+1)
	sum := sha256.Sum256(big)
	h := hex.EncodeToString(sum[:])
	_, err := tir.Export(src, "t-export", filepath.Join(t.TempDir(), "x.tir"), tir.ExportOptions{
		Mode:          tir.ModeFat,
		Artifacts:     []tir.ArtifactRef{{LogicalPath: "a.bin", ContentHash: h}},
		ArtifactBytes: map[string][]byte{h: big},
	})
	if !errors.Is(err, tir.ErrLimit) {
		t.Fatalf("err=%v want ErrLimit", err)
	}
}

func TestImportRejectsNilNodeLog(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "run.tir")
	path, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tir.Import(path, nil); !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	_, err := tir.Load(filepath.Join(t.TempDir(), "does-not-exist.tir"))
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestLoadRejectsPackageMissingRequiredMember(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	good := filepath.Join(t.TempDir(), "good.tir")
	if _, err := tir.Export(src, "t-export", good, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	incomplete := filepath.Join(t.TempDir(), "incomplete.tir")
	if err := dropZipMember(good, incomplete, "seals.json"); err != nil {
		t.Fatal(err)
	}
	_, err := tir.Load(incomplete)
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestLoadRejectsMalformedManifest(t *testing.T) {
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	good := filepath.Join(t.TempDir(), "good.tir")
	if _, err := tir.Export(src, "t-export", good, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad-manifest.tir")
	if err := replaceZipMember(good, bad, "manifest.json", []byte("not json")); err != nil {
		t.Fatal(err)
	}
	_, err := tir.Load(bad)
	if !errors.Is(err, tir.ErrTir) {
		t.Fatalf("err=%v want ErrTir", err)
	}
}

func TestImportPythonGoldenFixture(t *testing.T) {
	// Cross-language: package produced by Python reference (testdata/sample_thin.tir).
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	golden := filepath.Join(root, "testdata", "sample_thin.tir")
	if _, err := os.Stat(golden); err != nil {
		t.Fatalf("golden fixture missing (generate with scripts/gen_tir_fixture.py): %v", err)
	}
	dest := openLog(t, "from-py.sqlite")
	pkg, err := tir.Import(golden, dest)
	if err != nil {
		t.Fatalf("import python golden: %v", err)
	}
	if len(pkg.Nodes) < 1 {
		t.Fatal("empty package")
	}
	traj, _ := pkg.Manifest["trajectory_id"].(string)
	nodes, err := dest.ListNodesAllTenants(traj)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != len(pkg.Nodes) {
		t.Fatalf("imported %d want %d", len(nodes), len(pkg.Nodes))
	}
}

func rewriteNodes(src, dst string, mut func([][]byte) [][]byte) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()

	for _, zf := range r.File {
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if zf.Name == "nodes.ndjson" {
			rawLines := splitNonEmpty(data)
			rawLines = mut(rawLines)
			var buf []byte
			for _, ln := range rawLines {
				buf = append(buf, ln...)
				buf = append(buf, '\n')
			}
			data = buf
		}
		out, err := w.Create(zf.Name)
		if err != nil {
			return err
		}
		if _, err := out.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func injectZipMember(src, dst, name string, body []byte) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()

	for _, zf := range r.File {
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		out, err := w.Create(zf.Name)
		if err != nil {
			return err
		}
		if _, err := out.Write(data); err != nil {
			return err
		}
	}
	out, err := w.Create(name)
	if err != nil {
		return err
	}
	_, err = out.Write(body)
	return err
}

func dropZipMember(src, dst, name string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()

	for _, zf := range r.File {
		if zf.Name == name {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		out, err := w.Create(zf.Name)
		if err != nil {
			return err
		}
		if _, err := out.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func replaceZipMember(src, dst, name string, body []byte) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()

	for _, zf := range r.File {
		data := body
		if zf.Name != name {
			rc, err := zf.Open()
			if err != nil {
				return err
			}
			data, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
		}
		out, err := w.Create(zf.Name)
		if err != nil {
			return err
		}
		if _, err := out.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func splitNonEmpty(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, append([]byte(nil), data[start:i]...))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, append([]byte(nil), data[start:]...))
	}
	return lines
}
