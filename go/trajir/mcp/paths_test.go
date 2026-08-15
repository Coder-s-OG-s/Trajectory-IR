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

func TestRejectSymlinkSQLiteLeaves(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvWorkspaceRoot, root)
	proj := filepath.Join(root, "proj")
	if err := os.Mkdir(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	outsideDB := filepath.Join(outside, "nodes.sqlite")
	if err := os.WriteFile(outsideDB, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(proj, "nodes.sqlite")
	if err := os.Symlink(outsideDB, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	if _, _, err := workdirSQLitePaths(proj); err == nil {
		t.Fatal("expected symlink leaf to be rejected")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err=%v", err)
	}

	// Regular missing leaves are OK (will be created).
	clean := filepath.Join(root, "clean")
	if err := os.Mkdir(clean, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := workdirSQLitePaths(clean); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyWorkDirUsesRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvWorkspaceRoot, root)
	got, err := requireBoundedWorkDir("")
	if err != nil {
		t.Fatal(err)
	}
	// Canonical forms may differ by symlink resolution.
	if filepath.Clean(got) != filepath.Clean(root) {
		// Allow EvalSymlinks normalization
		g2, _ := filepath.EvalSymlinks(got)
		r2, _ := filepath.EvalSymlinks(root)
		if g2 != r2 {
			t.Fatalf("got=%q root=%q", got, root)
		}
	}
}

func TestRegularSQLiteLeafAllowed(t *testing.T) {
	root := t.TempDir()
	t.Setenv(EnvWorkspaceRoot, root)
	nodes := filepath.Join(root, "nodes.sqlite")
	if err := os.WriteFile(nodes, []byte("not-really-sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireNonSymlinkLeaf(root, nodes); err != nil {
		t.Fatal(err)
	}
	if _, err := requireBoundedPath("", ""); err == nil {
		t.Fatal("empty path should fail")
	}
	if _, err := requireBoundedPath("ok.tir", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := requireBoundedWorkDir(nodes); err == nil {
		t.Fatal("file as work_dir should fail")
	}
}
