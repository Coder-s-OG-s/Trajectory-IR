package tir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceFileHappyPath(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg.tir")
	src := filepath.Join(dir, "new.tmp")
	if err := os.WriteFile(dest, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("NEW"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(src, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW" {
		t.Fatalf("got %q want NEW", got)
	}
	if _, err := os.Stat(dest + ".trajir-replace-bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should be gone: %v", err)
	}
}

func TestReplaceFileViaBackupRestoresOriginal(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "pkg.tir")
	src := filepath.Join(dir, "new.tmp")
	if err := os.WriteFile(dest, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("NEW"), 0o644); err != nil {
		t.Fatal(err)
	}

	afterBackupHook = func(string) {
		// Simulate final rename failure (e.g. AV lock / access denied).
		_ = os.Remove(src)
	}
	defer func() { afterBackupHook = nil }()

	err := replaceFileViaBackup(src, dest)
	if err == nil {
		t.Fatal("expected replace failure")
	}
	if !strings.Contains(err.Error(), "replace package") {
		t.Fatalf("unexpected err: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("original package missing after failed replace: %v", err)
	}
	if string(got) != "ORIGINAL" {
		t.Fatalf("got %q want ORIGINAL (must not destroy package)", got)
	}
}

func TestInstallFileMissingDestKeepsRenameError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new.tmp")
	dest := filepath.Join(dir, "missing-subdir", "pkg.tir")
	if err := os.WriteFile(src, []byte("NEW"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := installFile(src, dest)
	if err == nil {
		t.Fatal("expected rename failure")
	}
	if strings.Contains(err.Error(), "no such file or directory") && strings.Contains(err.Error(), "move aside") {
		t.Fatalf("masked original rename error: %v", err)
	}
	if strings.Contains(err.Error(), "replace package (move aside)") {
		t.Fatalf("should not enter backup path when dest is missing: %v", err)
	}
}
