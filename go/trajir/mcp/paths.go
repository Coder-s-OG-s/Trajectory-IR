package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvWorkspaceRoot is the environment variable for the approved workspace root.
// When unset, the process working directory is used. All MCP tool paths must
// resolve under this root (CWE-73 / prompt-injected path confinement).
const EnvWorkspaceRoot = "TRAJIR_MCP_ROOT"

// approvedRoot returns the canonical absolute workspace root.
func approvedRoot() (string, error) {
	root := strings.TrimSpace(os.Getenv(EnvWorkspaceRoot))
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("mcp: resolve workspace root: %w", err)
		}
		root = wd
	}
	return canonicalizeExisting(root)
}

// requireBoundedWorkDir validates work_dir under the approved root.
// Empty work_dir means the root itself.
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
	// Allow paths under preferRoot if it is itself under root.
	base := root
	if strings.TrimSpace(preferRoot) != "" {
		pref, err := resolveUnderRoot(root, preferRoot, true)
		if err == nil {
			base = pref
		}
	}
	return resolveUnderRoot(base, path, false)
}

// resolveUnderRoot cleans and absolute-izes path, ensuring it stays under root.
// requireDir when true requires the path to be an existing directory (or creatable under root).
func resolveUnderRoot(root, userPath string, requireDir bool) (string, error) {
	rootAbs, err := canonicalizeExisting(root)
	if err != nil {
		return "", err
	}

	candidate := userPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	candidate = filepath.Clean(candidate)

	// For paths that may not exist yet (export dest), bound via parent.
	if _, err := os.Lstat(candidate); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("mcp: path %q: %w", userPath, err)
		}
		parent := filepath.Dir(candidate)
		parentAbs, err := canonicalizeExisting(parent)
		if err != nil {
			return "", fmt.Errorf("mcp: path parent %q not under workspace: %w", parent, err)
		}
		if !isSubpath(rootAbs, parentAbs) {
			return "", fmt.Errorf("mcp: path %q escapes workspace root %q", userPath, rootAbs)
		}
		// Rebuild absolute candidate under cleaned parent (no symlink escape on file itself yet).
		final := filepath.Join(parentAbs, filepath.Base(candidate))
		if requireDir {
			return "", fmt.Errorf("mcp: work_dir %q does not exist", userPath)
		}
		return final, nil
	}

	abs, err := canonicalizeExisting(candidate)
	if err != nil {
		return "", err
	}
	if !isSubpath(rootAbs, abs) {
		return "", fmt.Errorf("mcp: path %q escapes workspace root %q", userPath, rootAbs)
	}
	if requireDir {
		st, err := os.Stat(abs)
		if err != nil {
			return "", err
		}
		if !st.IsDir() {
			return "", fmt.Errorf("mcp: work_dir %q is not a directory", userPath)
		}
	}
	return abs, nil
}

func canonicalizeExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// EvalSymlinks fails if path does not exist; caller handles that case.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return filepath.Clean(abs), nil
		}
		return "", err
	}
	return resolved, nil
}

func isSubpath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	// Windows volume mismatches yield paths like `..\..`
	return !strings.HasPrefix(rel, "..")
}
