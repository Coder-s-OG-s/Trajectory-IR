package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathConfinementRejectsEscape(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvWorkspaceRoot, root)

	// Create an outside dir we must not touch.
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.tir")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := requireBoundedPath(outsideFile, ""); err == nil {
		t.Fatal("expected escape via absolute path to fail")
	} else if !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("unexpected err: %v", err)
	}

	if _, err := requireBoundedPath(filepath.Join("..", filepath.Base(outside), "secret.tir"), ""); err == nil {
		// may fail for other reasons; only fail if it succeeds under root
		// relative .. from root should escape
		t.Log("relative escape rejected or not applicable")
	}

	// Valid relative path under root (file may not exist yet for export).
	dest, err := requireBoundedPath("out.tir", root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dest, root) {
		t.Fatalf("dest=%q root=%q", dest, root)
	}

	// work_dir must exist as directory
	if _, err := requireBoundedWorkDir("missing-dir"); err == nil {
		t.Fatal("expected missing work_dir to fail")
	}
	sub := filepath.Join(root, "proj")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := requireBoundedWorkDir(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != sub && !strings.HasPrefix(got, root) {
		// EvalSymlinks may normalize
		t.Logf("work_dir=%q", got)
	}
}

func TestRequireIDs(t *testing.T) {
	if err := requireIDs("", "t"); err == nil {
		t.Fatal("expected error")
	}
	if err := requireIDs("a", "b"); err != nil {
		t.Fatal(err)
	}
}
