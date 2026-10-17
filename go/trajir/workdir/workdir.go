// Package workdir provides generic, confinement-policy-free filesystem
// helpers for resolving user-supplied paths safely: canonicalizing
// directories, walking up to the nearest existing ancestor before
// resolving symlinks (so a symlinked parent can't be combined with a
// nonexistent leaf path to land somewhere unexpected), checking subpath
// containment, and rejecting symlinked leaf files. This package has no
// opinion on which root a caller should confine paths to — callers that
// need to enforce a specific confinement policy (an approved-root env
// var, for example) build it on top of these primitives.
package workdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalizeDir resolves path to an absolute, symlink-resolved directory
// path. It returns an error if path does not exist or is not a directory.
func CanonicalizeDir(path string) (string, error) {
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

// ResolveViaExistingAncestor finds the nearest existing ancestor of path,
// resolves symlinks there with EvalSymlinks, then rejoins any missing
// trailing path components. This blocks a symlinked/junction parent from
// being combined with a nonexistent leaf path to resolve somewhere other
// than where the symlink target actually points.
func ResolveViaExistingAncestor(path string) (string, error) {
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

// IsSubpath reports whether target is root itself or lies inside it. A
// name that merely starts with ".." (e.g. "..secrets") is not treated as
// parent traversal; only an actual ".." path segment counts as escaping
// root.
func IsSubpath(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// RequireNonSymlinkLeaf rejects a leafPath that is itself a symlink, or
// whose resolved location escapes root. A leafPath that does not exist yet
// is allowed, since callers typically create it as a regular file on
// first use.
func RequireNonSymlinkLeaf(root, leafPath string) error {
	info, err := os.Lstat(leafPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("workdir: inspect %q: %w", filepath.Base(leafPath), err)
		}
	} else if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workdir: %q must not be a symlink", filepath.Base(leafPath))
	}
	rootCanon, err := ResolveViaExistingAncestor(root)
	if err != nil {
		return fmt.Errorf("workdir: resolve root %q: %w", root, err)
	}
	abs, err := ResolveViaExistingAncestor(leafPath)
	if err != nil {
		return err
	}
	if !IsSubpath(rootCanon, abs) {
		return fmt.Errorf("workdir: %q escapes root %q", filepath.Base(leafPath), root)
	}
	return nil
}
