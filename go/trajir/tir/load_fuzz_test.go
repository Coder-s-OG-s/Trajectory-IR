package tir_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

func FuzzLoadPackage(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte("PK\x03\x04"))
	f.Add(fuzzZip(f, []fuzzMember{
		{name: "../escape", body: "x"},
	}))
	f.Add(fuzzZip(f, []fuzzMember{
		{name: "manifest.json", body: `{"mode":"thin","tenant_id":"demo"}`},
		{name: "nodes.ndjson", body: "not-json\n"},
		{name: "seals.json", body: "[]\n"},
		{name: "artifacts-manifest.json", body: "[]\n"},
		{name: "COMPAT.json", body: "{}\n"},
	}))
	f.Add(fuzzThinPackage(f))
	f.Fuzz(func(t *testing.T, raw []byte) {
		pkg, err := tir.LoadReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			if pkg != nil {
				t.Fatalf("package returned with error: %v", err)
			}
			return
		}
		if pkg == nil {
			t.Fatal("nil package")
		}
		if len(pkg.Nodes) == 0 || len(pkg.Nodes) > tir.MaxNodes {
			t.Fatalf("nodes=%d", len(pkg.Nodes))
		}
		if len(pkg.ArtifactsManifest) > tir.MaxArtifacts || len(pkg.ArtifactBytes) > tir.MaxArtifacts {
			t.Fatalf("artifacts manifest=%d embedded=%d", len(pkg.ArtifactsManifest), len(pkg.ArtifactBytes))
		}
	})
}

type fuzzMember struct {
	name string
	body string
}

func fuzzZip(f *testing.F, members []fuzzMember) []byte {
	f.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, m := range members {
		fw, err := w.Create(m.name)
		if err != nil {
			f.Fatal(err)
		}
		if _, err := fw.Write([]byte(m.body)); err != nil {
			f.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		f.Fatal(err)
	}
	return buf.Bytes()
}

func fuzzThinPackage(f *testing.F) []byte {
	f.Helper()
	dir := f.TempDir()
	nl, err := nodelog.Open(filepath.Join(dir, "seed.sqlite"))
	if err != nil {
		f.Fatal(err)
	}
	defer func() { _ = nl.Close() }()
	step := 1
	if _, err := nl.Append("PROJECT_CONTEXT", &step, map[string]any{"goal": "demo"}, "t-fuzz", "demo", 0); err != nil {
		f.Fatal(err)
	}
	out := filepath.Join(dir, "seed.tir")
	path, err := tir.Export(nl, "t-fuzz", out, tir.ExportOptions{Mode: tir.ModeThin})
	if err != nil {
		f.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		f.Fatal(err)
	}
	return b
}
