package tir_test

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/audit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

// rewriteFormat copies a package and sets package_format in manifest.json and
// COMPAT.json. An empty value deletes the key.
func rewriteFormat(t *testing.T, src, manifestFormat, compatFormat string) string {
	t.Helper()
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	dst := filepath.Join(t.TempDir(), "rewritten.tir")
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, m := range zr.File {
		rc, err := m.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		switch m.Name {
		case "manifest.json":
			data = setFormat(t, data, manifestFormat)
		case "COMPAT.json":
			data = setFormat(t, data, compatFormat)
		}
		w, err := zw.Create(m.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return dst
}

func setFormat(t *testing.T, raw []byte, v string) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if v == "" {
		delete(doc, "package_format")
	} else {
		doc["package_format"] = v
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func samplePackage(t *testing.T) string {
	t.Helper()
	src := openLog(t, "src.sqlite")
	seedSample(t, src)
	out := filepath.Join(t.TempDir(), "run.tir")
	if _, err := tir.Export(src, "t-export", out, tir.ExportOptions{Mode: tir.ModeThin}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLoadRejectsUnknownFormatMajor(t *testing.T) {
	pkg := rewriteFormat(t, samplePackage(t), "1.0", "1.0")
	_, err := tir.Load(pkg)
	if !errors.Is(err, tir.ErrFormat) {
		t.Fatalf("err=%v, want ErrFormat", err)
	}
	if !strings.Contains(err.Error(), "1.0") || !strings.Contains(err.Error(), tir.PackageFormatVersion) {
		t.Fatalf("error should name both versions: %v", err)
	}
	t.Setenv("TRAJIR_ALLOW_UNVERIFIED", "1")
	if _, err := tir.LoadUnverified(pkg); !errors.Is(err, tir.ErrFormat) {
		t.Fatalf("LoadUnverified err=%v, want ErrFormat", err)
	}
}

func TestLoadAcceptsNewerMinor(t *testing.T) {
	pkg := rewriteFormat(t, samplePackage(t), "0.9", "0.9")
	if _, err := tir.Load(pkg); err != nil {
		t.Fatalf("newer minor of the same major should load: %v", err)
	}
}

func TestLoadRejectsBadFormatFields(t *testing.T) {
	cases := map[string][2]string{
		"manifest and COMPAT disagree": {"0.1", "0.2"},
		"missing in COMPAT":            {"0.1", ""},
		"missing in manifest":          {"", "0.1"},
		"three parts":                  {"0.1.0", "0.1.0"},
		"not numeric":                  {"v0.1", "v0.1"},
	}
	src := samplePackage(t)
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := tir.Load(rewriteFormat(t, src, c[0], c[1]))
			if !errors.Is(err, tir.ErrFormat) {
				t.Fatalf("err=%v, want ErrFormat", err)
			}
		})
	}
}

func TestVerifyReportsUnsupportedFormat(t *testing.T) {
	pkg := rewriteFormat(t, samplePackage(t), "9.9", "9.9")
	res, err := audit.VerifyFile(pkg, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || len(res.Findings) != 1 || res.Findings[0].Code != "UNSUPPORTED_FORMAT" {
		t.Fatalf("want one UNSUPPORTED_FORMAT finding, got ok=%v %v", res.OK, res.Findings)
	}
}
