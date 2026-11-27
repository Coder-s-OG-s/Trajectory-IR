package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/workdir"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func fileURI(abs string) string {
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return "file://" + slash
}

func TestApprovedRootIgnoresProcessDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv(EnvWorkspaceRoot, "")
	t.Setenv(claudeProjectDirEnv, "")
	if _, err := approvedRoot(context.Background()); err == nil {
		t.Fatal("process directory was accepted as the workspace root")
	}
}

func TestClaudeProjectDirUsedWhenRootUnset(t *testing.T) {
	dir, err := workdir.CanonicalizeDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvWorkspaceRoot, "")
	t.Setenv(claudeProjectDirEnv, dir)
	got, err := approvedRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("root = %q, want %q", got, dir)
	}
}

func TestRelativeClaudeProjectDirIsIgnored(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EnvWorkspaceRoot, "")
	t.Setenv(claudeProjectDirEnv, ".")
	if _, err := approvedRoot(context.Background()); err == nil {
		t.Fatal("relative CLAUDE_PROJECT_DIR was accepted")
	}
}

func TestConfiguredRootWinsOverClaudeProjectDir(t *testing.T) {
	configured, err := workdir.CanonicalizeDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvWorkspaceRoot, configured)
	t.Setenv(claudeProjectDirEnv, t.TempDir())
	got, err := approvedRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != configured {
		t.Fatalf("root = %q, want %q", got, configured)
	}
}

func TestUnexpandedRootPlaceholderUsesClaudeProjectDir(t *testing.T) {
	dir, err := workdir.CanonicalizeDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvWorkspaceRoot, "${CLAUDE_PROJECT_DIR}")
	t.Setenv(claudeProjectDirEnv, dir)
	got, err := approvedRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("root = %q, want %q", got, dir)
	}
}

func TestFileURIRoundTrip(t *testing.T) {
	dir, err := workdir.CanonicalizeDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{fileURI(dir), "file://localhost" + fileURI(dir)[len("file://"):]} {
		got, err := pathFromFileURI(uri)
		if err != nil {
			t.Fatalf("%s: %v", uri, err)
		}
		canonical, err := workdir.CanonicalizeDir(got)
		if err != nil {
			t.Fatal(err)
		}
		if canonical != dir {
			t.Fatalf("%s resolved to %q, want %q", uri, canonical, dir)
		}
	}
}

func TestFileURIRejectsRemoteHost(t *testing.T) {
	if _, err := pathFromFileURI("file://example.com/tmp/project"); err == nil {
		t.Fatal("remote file host was accepted")
	}
}

func TestSingleClientRootRequired(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	one, err := singleWorkspaceRoot([]*mcp.Root{{URI: fileURI(dir)}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := workdir.CanonicalizeDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if one != want {
		t.Fatalf("root = %q, want %q", one, want)
	}
	if _, err := singleWorkspaceRoot(nil); err == nil {
		t.Fatal("empty root list was accepted")
	}
	if _, err := singleWorkspaceRoot([]*mcp.Root{{URI: fileURI(dir)}, {URI: fileURI(other)}}); err == nil {
		t.Fatal("two workspace directories were accepted")
	}
}
