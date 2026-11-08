package tir_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

func FuzzLoadPackage(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte("PK\x03\x04"))
	f.Add(fuzzZip(f, []fuzzMember{
		{name: "../escape", body: []byte("x")},
	}))
	f.Add(fuzzZip(f, []fuzzMember{
		{name: "manifest.json", body: []byte(`{"mode":"thin","tenant_id":"demo"}`)},
		{name: "nodes.ndjson", body: []byte("not-json\n")},
		{name: "seals.json", body: []byte("[]\n")},
		{name: "artifacts-manifest.json", body: []byte("[]\n")},
		{name: "COMPAT.json", body: []byte("{}\n")},
	}))
	f.Add(fuzzSeed(f, "thin-decision.tir"))
	f.Add(fuzzSeed(f, "fat-cas.tir"))
	f.Add(fuzzSeed(f, "signed-thin.tir"))
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
		if len(pkg.Seals) > tir.MaxSeals {
			t.Fatalf("seals=%d", len(pkg.Seals))
		}
		if len(pkg.ArtifactsManifest) > tir.MaxArtifacts || len(pkg.ArtifactBytes) > tir.MaxArtifacts {
			t.Fatalf("artifacts manifest=%d embedded=%d", len(pkg.ArtifactsManifest), len(pkg.ArtifactBytes))
		}
		if err := loadedPackageInvariants(pkg); err != nil {
			t.Fatal(err)
		}
	})
}

func TestFuzzSeedsCoverLoadPaths(t *testing.T) {
	thinRaw := fuzzSeed(t, "thin-decision.tir")
	thin, err := tir.LoadReader(bytes.NewReader(thinRaw), int64(len(thinRaw)))
	if err != nil {
		t.Fatal(err)
	}
	if !nodeKind(thin, "DECISION") || len(thin.Seals) == 0 {
		t.Fatalf("decision seed nodes=%d seals=%d", len(thin.Nodes), len(thin.Seals))
	}
	if thin.Manifest["mode"] != string(tir.ModeThin) || len(thin.ArtifactBytes) != 0 || thin.Signature != nil {
		t.Fatalf("decision seed mode=%v embedded=%d sig=%v", thin.Manifest["mode"], len(thin.ArtifactBytes), thin.Signature != nil)
	}

	fatRaw := fuzzSeed(t, "fat-cas.tir")
	fat, err := tir.LoadReader(bytes.NewReader(fatRaw), int64(len(fatRaw)))
	if err != nil {
		t.Fatal(err)
	}
	if fat.Manifest["mode"] != string(tir.ModeFat) || len(fat.ArtifactBytes) == 0 || !zipHasPrefix(t, fatRaw, "artifacts/cas/") {
		t.Fatalf("fat seed mode=%v embedded=%d", fat.Manifest["mode"], len(fat.ArtifactBytes))
	}

	signedRaw := fuzzSeed(t, "signed-thin.tir")
	signed, err := tir.LoadReader(bytes.NewReader(signedRaw), int64(len(signedRaw)))
	if err != nil {
		t.Fatal(err)
	}
	if signed.Signature == nil || !zipHasName(t, signedRaw, "SIGNATURE") {
		t.Fatal("signed seed has no signature")
	}
	if signed.Manifest["mode"] != string(tir.ModeThin) || len(signed.ArtifactBytes) != 0 {
		t.Fatalf("signed seed mode=%v embedded=%d", signed.Manifest["mode"], len(signed.ArtifactBytes))
	}
	if !nodeKind(signed, "DECISION") || len(signed.Seals) == 0 {
		t.Fatalf("signed seed nodes=%d seals=%d", len(signed.Nodes), len(signed.Seals))
	}
}

func TestLoadRejectsTooManySeals(t *testing.T) {
	var body []byte
	body = append(body, '[')
	one := []byte(`{"node_id":"x"}`)
	for i := 0; i < tir.MaxSeals+1; i++ {
		if i > 0 {
			body = append(body, ',')
		}
		body = append(body, one...)
	}
	body = append(body, ']')
	raw := fuzzZip(t, []fuzzMember{
		{name: "manifest.json", body: []byte(`{"mode":"thin","tenant_id":"demo"}`)},
		{name: "nodes.ndjson", body: []byte("\n")},
		{name: "seals.json", body: body},
		{name: "artifacts-manifest.json", body: []byte("[]")},
		{name: "COMPAT.json", body: []byte("{}")},
	})
	_, err := tir.LoadReader(bytes.NewReader(raw), int64(len(raw)))
	if !errors.Is(err, tir.ErrLimit) || !strings.Contains(err.Error(), "too many seals") {
		t.Fatalf("err=%v", err)
	}
}

type fuzzMember struct {
	name string
	body []byte
}

type testFatal interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func fuzzZip(tb testFatal, members []fuzzMember) []byte {
	tb.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, m := range members {
		fw, err := w.Create(m.name)
		if err != nil {
			tb.Fatal(err)
		}
		if _, err := fw.Write(m.body); err != nil {
			tb.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		tb.Fatal(err)
	}
	return buf.Bytes()
}

func fuzzSeed(tb testFatal, name string) []byte {
	tb.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "fuzz-seeds", name))
	if err != nil {
		tb.Fatal(err)
	}
	if len(b) == 0 {
		tb.Fatalf("empty seed %s", name)
	}
	return b
}

func loadedPackageInvariants(pkg *tir.Package) error {
	var seen string
	have := false
	for _, n := range pkg.Nodes {
		nt, _ := n["tenant_id"].(string)
		if !have {
			seen = nt
			have = true
			continue
		}
		if nt != seen {
			return errors.New("mixed tenant_id")
		}
	}
	if manifestTenant, ok := pkg.Manifest["tenant_id"].(string); ok && manifestTenant != "" && manifestTenant != seen {
		return errors.New("manifest tenant_id does not match nodes")
	}
	mode, _ := pkg.Manifest["mode"].(string)
	if mode == "" {
		mode = string(tir.ModeThin)
	}
	if mode == string(tir.ModeThin) && len(pkg.ArtifactBytes) != 0 {
		return errors.New("thin package embedded artifact bytes")
	}
	if mode == string(tir.ModeFat) {
		for _, entry := range pkg.ArtifactsManifest {
			h, _ := entry["content_hash"].(string)
			if _, ok := pkg.ArtifactBytes[h]; !ok {
				return errors.New("fat package missing artifact bytes")
			}
		}
	}
	return nil
}

func nodeKind(pkg *tir.Package, kind string) bool {
	for _, n := range pkg.Nodes {
		if k, _ := n["kind"].(string); k == kind {
			return true
		}
	}
	return false
}

func zipHasPrefix(tb testFatal, raw []byte, prefix string) bool {
	tb.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		tb.Fatal(err)
	}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, prefix) {
			return true
		}
	}
	return false
}

func zipHasName(tb testFatal, raw []byte, name string) bool {
	tb.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		tb.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}
