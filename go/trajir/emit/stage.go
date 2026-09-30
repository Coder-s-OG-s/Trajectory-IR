package emit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	// PackagesDir is the console-local copy of exported .tir files.
	PackagesDir = "packages"
	// MaxStageBytes caps a copied package so a bad path cannot fill the disk.
	MaxStageBytes int64 = 64 << 20
)

// StagePackage copies srcPath into dataDir/packages/<trajectoryID>/<name>.tir.
// The returned path uses forward slashes (console_path). A missing source is
// an error; callers must not fail the durable path on that error.
func StagePackage(dataDir, trajectoryID, srcPath, eventID string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", errors.New("emit: data dir required")
	}
	if err := validTrajectoryID(trajectoryID); err != nil {
		return "", err
	}
	srcPath = strings.TrimSpace(srcPath)
	if err := rejectUnsafePath(srcPath); err != nil {
		return "", err
	}
	src, err := filepath.Abs(srcPath)
	if err != nil {
		return "", err
	}
	if err := rejectUnsafePath(src); err != nil {
		return "", err
	}
	info, err := lstatChecked(src)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("emit: package is not a regular file")
	}
	if info.Size() > MaxStageBytes {
		return "", fmt.Errorf("emit: package too large (%d bytes)", info.Size())
	}
	name, err := SafeTirName(filepath.Base(src))
	if err != nil {
		if eventID == "" {
			return "", err
		}
		name, err = SafeTirName(sanitizeID(eventID) + ".tir")
		if err != nil {
			return "", err
		}
	}
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	destDir := filepath.Join(root, PackagesDir, trajectoryID)
	if !underRoot(root, destDir) {
		return "", errors.New("emit: dest escapes data dir")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, name)
	if !underRoot(root, dest) {
		return "", errors.New("emit: dest escapes data dir")
	}
	if di, err := lstatChecked(dest); err == nil {
		if di.Mode().IsRegular() && di.Size() == info.Size() {
			same, sameErr := sameRegularFile(src, dest)
			if sameErr == nil && same {
				return RelConsolePath(trajectoryID, name), nil
			}
		}
		stem := strings.TrimSuffix(name, ".tir")
		suffix := sanitizeID(eventID)
		if suffix == "" {
			suffix = "dup"
		}
		if len(suffix) > 12 {
			suffix = suffix[:12]
		}
		name = stem + "-" + suffix + ".tir"
		if _, err := SafeTirName(name); err != nil {
			return "", err
		}
		dest = filepath.Join(destDir, name)
		if !underRoot(root, dest) {
			return "", errors.New("emit: dest escapes data dir")
		}
	}
	if err := copyRegularFile(src, dest); err != nil {
		return "", err
	}
	return RelConsolePath(trajectoryID, name), nil
}

// RelConsolePath is packages/<id>/<name> with forward slashes.
func RelConsolePath(trajectoryID, name string) string {
	return PackagesDir + "/" + trajectoryID + "/" + name
}

// SafeTirName accepts a basename that ends in .tir.
func SafeTirName(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" || base != filepath.Base(base) {
		return "", errors.New("emit: bad package name")
	}
	if strings.Contains(base, "..") || strings.ContainsAny(base, `/\`) {
		return "", errors.New("emit: bad package name")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".tir") {
		return "", errors.New("emit: package name must end in .tir")
	}
	if len(base) > 180 {
		return "", errors.New("emit: package name too long")
	}
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return "", errors.New("emit: bad package name")
	}
	return base, nil
}

func sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// rejectUnsafePath is a CodeQL-recognized sanitizer: empty, NUL, and ".."
// are refused on the same string later passed to Lstat/Open.
func rejectUnsafePath(p string) error {
	if p == "" {
		return errors.New("emit: package path required")
	}
	if strings.IndexByte(p, 0) >= 0 {
		return errors.New("emit: bad package path")
	}
	if strings.Contains(p, "..") {
		return errors.New("emit: bad package path")
	}
	return nil
}

func underRoot(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, candAbs)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func lstatChecked(path string) (os.FileInfo, error) {
	if err := rejectUnsafePath(path); err != nil {
		return nil, err
	}
	return os.Lstat(path)
}

func openChecked(path string) (*os.File, error) {
	if err := rejectUnsafePath(path); err != nil {
		return nil, err
	}
	return os.Open(path)
}

func sameRegularFile(a, b string) (bool, error) {
	fa, err := openChecked(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := openChecked(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	bufA := make([]byte, 32*1024)
	bufB := make([]byte, 32*1024)
	for {
		na, errA := io.ReadFull(fa, bufA)
		nb, errB := io.ReadFull(fb, bufB)
		if na != nb {
			return false, nil
		}
		if na > 0 && string(bufA[:na]) != string(bufB[:nb]) {
			return false, nil
		}
		eofA := errors.Is(errA, io.EOF) || errors.Is(errA, io.ErrUnexpectedEOF)
		eofB := errors.Is(errB, io.EOF) || errors.Is(errB, io.ErrUnexpectedEOF)
		if eofA && eofB {
			return true, nil
		}
		if errA != nil && !eofA {
			return false, errA
		}
		if errB != nil && !eofB {
			return false, errB
		}
		if eofA != eofB {
			return false, nil
		}
	}
}

func copyRegularFile(src, dest string) error {
	in, err := openChecked(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := rejectUnsafePath(dest); err != nil {
		return err
	}
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(in, MaxStageBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
