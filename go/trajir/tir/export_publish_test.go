package tir

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

func exportSampleLog(t *testing.T) *nodelog.NodeLog {
	t.Helper()
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })
	step := 1
	if _, err := nl.Append("DECISION", &step, map[string]any{"plan": map[string]any{}}, "t-export", "demo", 1); err != nil {
		t.Fatal(err)
	}
	return nl
}

func testSignKey() ed25519.PrivateKey {
	sum := sha256.Sum256([]byte("trajir-pkg-sig-v1-test-vector-seed"))
	return ed25519.NewKeyFromSeed(sum[:])
}

func TestExportSignFailureRemovesUnsignedPackage(t *testing.T) {
	nl := exportSampleLog(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.tir")
	if err := os.WriteFile(out, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}

	signExport = func(path string, _ ed25519.PrivateKey, _ SignerMeta) error {
		zr, zerr := zip.OpenReader(path)
		if zerr != nil {
			return zerr
		}
		names := map[string]bool{}
		for _, f := range zr.File {
			names[f.Name] = true
		}
		_ = zr.Close()
		if names[SignatureMemberName] {
			return errors.New("signature already present")
		}
		for _, name := range requiredMembers {
			if !names[name] {
				return errors.New("missing " + name)
			}
		}
		return errors.New("disk full")
	}
	defer func() { signExport = Sign }()

	var notice ExportNotice
	sawFile := false
	_, err := Export(nl, "t-export", out, ExportOptions{
		Mode:    ModeThin,
		SignKey: testSignKey(),
		OnExported: func(n ExportNotice) {
			notice = n
			_, statErr := os.Stat(n.Path)
			sawFile = statErr == nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err=%v", err)
	}
	if notice.OK || !strings.Contains(notice.Error, "disk full") || notice.Bytes <= 0 || !sawFile {
		t.Fatalf("notice=%+v sawFile=%v", notice, sawFile)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Fatalf("dest after failed sign: %v", statErr)
	}
	left, err := filepath.Glob(filepath.Join(dir, ".tir-export-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("temp left behind: %v", left)
	}
}

func TestExportKeepsExistingFileWhenPublishFails(t *testing.T) {
	nl := exportSampleLog(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "keep.tir")
	if err := os.WriteFile(out, []byte("OLD-PACKAGE"), 0o644); err != nil {
		t.Fatal(err)
	}

	beforeExportReplace = func() error {
		matches, globErr := filepath.Glob(filepath.Join(dir, ".tir-export-*"))
		if globErr != nil {
			return globErr
		}
		if len(matches) != 1 {
			return errors.New("export temp missing")
		}
		got, readErr := os.ReadFile(out)
		if readErr != nil {
			return readErr
		}
		if string(got) != "OLD-PACKAGE" {
			return errors.New("destination changed before publish")
		}
		return errors.New("disk full")
	}
	defer func() { beforeExportReplace = nil }()

	_, err := Export(nl, "t-export", out, ExportOptions{Mode: ModeThin})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err=%v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "OLD-PACKAGE" {
		t.Fatalf("dest=%q", got)
	}
	left, err := filepath.Glob(filepath.Join(dir, ".tir-export-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("temp left behind: %v", left)
	}
}

func TestExportOverwriteKeepsLoadablePackage(t *testing.T) {
	nl := exportSampleLog(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "out.tir")
	if err := os.WriteFile(out, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(nl, "t-export", out, ExportOptions{Mode: ModeThin}); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out + ".trajir-replace-bak"); !os.IsNotExist(err) {
		t.Fatalf("backup left behind: %v", err)
	}
	left, err := filepath.Glob(filepath.Join(dir, ".tir-export-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("temp left behind: %v", left)
	}
}

func TestExportSignedOverwriteHasSignature(t *testing.T) {
	nl := exportSampleLog(t)
	out := filepath.Join(t.TempDir(), "out.tir")
	if err := os.WriteFile(out, []byte("OLD"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := Export(nl, "t-export", out, ExportOptions{Mode: ModeThin, SignKey: testSignKey()})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Signature == nil {
		t.Fatal("missing signature")
	}
	if _, err := os.Stat(out + ".trajir-replace-bak"); !os.IsNotExist(err) {
		t.Fatalf("backup left behind: %v", err)
	}
	dir := filepath.Dir(out)
	for _, pat := range []string{".tir-export-*", "tir-sign-*.tmp"} {
		left, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			t.Fatal(err)
		}
		if len(left) != 0 {
			t.Fatalf("temp left behind: %v", left)
		}
	}
}
