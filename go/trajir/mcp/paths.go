package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvWorkspaceRoot is the environment variable for the approved workspace root.
// It must be set explicitly: all MCP tool paths must resolve under this root
// (CWE-73 / prompt-injected path confinement), so silently falling back to the
// process's cwd when unset would let confinement widen to whatever directory
// happened to launch the binary. Fail closed instead.
const EnvWorkspaceRoot = "TRAJIR_MCP_ROOT"

// approvedRoot returns the canonical absolute workspace root.
func approvedRoot() (string, error) {
	root := strings.TrimSpace(os.Getenv(EnvWorkspaceRoot))
	if root == "" {
		return "", fmt.Errorf("mcp: %s must be set to an approved workspace root", EnvWorkspaceRoot)
	}
	return canonicalizeDir(root)
}

// requireBoundedWorkDir validates work_dir under the approved root.
// Empty work_dir means the root itself. The directory must already exist.
func requireBoundedWorkDir(workDir string) (string, error) {
	root, err := approvedRoot()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(workDir) == "" {
		return root, nil
	}
	return resolveUnderRoot(root, workDir, true)
}

// requireBoundedPath validates path is under root (or under preferredRoot when set).
// preferRoot is typically the validated work_dir for exports.
func requireBoundedPath(path, preferRoot string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("mcp: path is required")
	}
	root, err := approvedRoot()
	if err != nil {
		return "", err
	}
	base := root
	if strings.TrimSpace(preferRoot) != "" {
		pref, err := resolveUnderRoot(root, preferRoot, true)
		if err != nil {
			return "", err
		}
		base = pref
	}
	return resolveUnderRoot(base, path, false)
}

// resolveUnderRoot cleans and absolute-izes path, ensuring it stays under root.
// When requireDir is true, path must already exist and be a directory.
func resolveUnderRoot(root, userPath string, requireDir bool) (string, error) {
	rootAbs, err := canonicalizeDir(root)
	if err != nil {
		return "", err
	}

	candidate := userPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	candidate = filepath.Clean(candidate)

	resolved, err := resolveViaExistingAncestor(candidate)
	if err != nil {
		return "", fmt.Errorf("mcp: path %q: %w", userPath, err)
	}
	if !isSubpath(rootAbs, resolved) {
		return "", fmt.Errorf("mcp: path %q escapes workspace root %q", userPath, rootAbs)
	}
	if requireDir {
		st, err := os.Stat(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("mcp: work_dir %q does not exist", userPath)
			}
			return "", err
		}
		if !st.IsDir() {
			return "", fmt.Errorf("mcp: work_dir %q is not a directory", userPath)
		}
	}
	return resolved, nil
}

// resolveViaExistingAncestor finds the nearest existing ancestor, resolves
// symlinks there with EvalSymlinks, then rejoins any missing trailing components.
// This blocks junction/symlink parents used with a nonexistent leaf path.
func resolveViaExistingAncestor(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	var missing []string
	cur := abs
	for {
		_, err := os.Lstat(cur)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		base := filepath.Base(cur)
		missing = append([]string{base}, missing...)
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("no existing ancestor")
		}
		cur = parent
	}

	resolvedAncestor, err := filepath.EvalSymlinks(cur)
	if err != nil {
		return "", err
	}
	if len(missing) == 0 {
		return resolvedAncestor, nil
	}
	return filepath.Join(append([]string{resolvedAncestor}, missing...)...), nil
}

func canonicalizeDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("not a directory: %s", path)
	}
	return resolved, nil
}

func isSubpath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	// Only treat true parent traversal as escape, not names like "..secrets".
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// requireNonSymlinkLeaf rejects symlinked files under an approved directory.
// Missing files are allowed (callers create regular files on open).
// Existing regular files are re-checked so their resolved path stays under root.
func requireNonSymlinkLeaf(root, leafPath string) error {
	info, err := os.Lstat(leafPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("mcp: inspect %q: %w", filepath.Base(leafPath), err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("mcp: %q must not be a symlink", filepath.Base(leafPath))
	}
	abs, err := resolveViaExistingAncestor(leafPath)
	if err != nil {
		return err
	}
	if !isSubpath(root, abs) {
		return fmt.Errorf("mcp: %q escapes workspace root %q", filepath.Base(leafPath), root)
	}
	return nil
}

// workdirSQLitePaths returns validated nodes/memo paths under workDir (no symlink leaves).
func workdirSQLitePaths(workDir string) (nodesPath, memoPath string, err error) {
	root, err := approvedRoot()
	if err != nil {
		return "", "", err
	}
	nodesPath = filepath.Join(workDir, "nodes.sqlite")
	memoPath = filepath.Join(workDir, "memo.sqlite")
	if err := requireNonSymlinkLeaf(root, nodesPath); err != nil {
		return "", "", err
	}
	if err := requireNonSymlinkLeaf(root, memoPath); err != nil {
		return "", "", err
	}
	return nodesPath, memoPath, nil
}
