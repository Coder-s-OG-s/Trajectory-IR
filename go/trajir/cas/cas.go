// Package cas implements content addressed artifact storage for the local
// profile (README section 11.2).
//
// Key layout matches the Python reference (trajectory_ir.storage):
//
//	cas/<first-2-hex-of-sha256>/<full-sha256-hex>
//
// Objects are identified solely by SHA-256 of their bytes. Get always
// re-hashes stored data and fails closed on mismatch.
package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Sentinel errors.
var (
	ErrCAS       = errors.New("cas")
	ErrNotFound  = errors.New("cas not found")
	ErrIntegrity = errors.New("cas integrity")
	ErrInvalid   = errors.New("cas invalid hash")
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Matches CreateTemp names: ".{64-hex}." + random suffix (same as Python mkstemp).
// ContentHash / NormalizeHash always use lowercase hex, so Put prefixes are
// lowercase only; matching A-F would not find writer temps and is unused.
var tempNameRe = regexp.MustCompile(`^\.[0-9a-f]{64}\.`)

// DefaultTempMaxAge is how old a Put temp must be before SweepStaleTempFiles
// will delete it.
const DefaultTempMaxAge = 24 * time.Hour

// Store is the minimal CAS surface used by thin package rehydrate.
type Store interface {
	Put(data []byte) (contentHash string, err error)
	Get(contentHash string) ([]byte, error)
	Has(contentHash string) bool
}

// ContentHash returns lowercase hex SHA-256 of data.
func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// NormalizeHash validates and lowercases a 64 hex content hash.
func NormalizeHash(h string) (string, error) {
	h = strings.ToLower(strings.TrimSpace(h))
	if !hex64.MatchString(h) {
		return "", fmt.Errorf("%w: need 64 lowercase hex digits, got %q", ErrInvalid, h)
	}
	return h, nil
}

// ShardKey returns the relative object key cas/<2hex>/<fullhash>.
func ShardKey(contentHash string) (string, error) {
	h, err := NormalizeHash(contentHash)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("cas", h[:2], h)), nil
}

// FileSystem is a directory rooted CAS for the local deployment profile.
type FileSystem struct {
	Root string
}

// NewFileSystem creates root if needed and returns a FileSystem store.
// It does not sweep orphaned Put temps; call SweepStaleTempFiles when no
// concurrent writers share this root (avoids deleting an in-flight temp).
func NewFileSystem(root string) (*FileSystem, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: root is required", ErrCAS)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("%w: mkdir root: %v", ErrCAS, err)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &FileSystem{Root: abs}, nil
}

// SweepStaleTempFiles removes orphaned Put temp files under cas/ older than maxAge.
// Only names matching the CreateTemp prefix (".{lowercase-hex64}.…") are considered.
// Pass DefaultTempMaxAge for the usual 24h threshold.
func (fs *FileSystem) SweepStaleTempFiles(maxAge time.Duration) {
	casRoot := filepath.Join(fs.Root, "cas")
	st, err := os.Stat(casRoot)
	if err != nil || !st.IsDir() {
		return
	}
	now := time.Now()
	_ = filepath.WalkDir(casRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !tempNameRe.MatchString(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if now.Sub(info.ModTime()) > maxAge {
			_ = os.Remove(path)
		}
		return nil
	})
}

// PathFor returns the absolute path for a content hash under this store.
func (fs *FileSystem) PathFor(contentHash string) (string, error) {
	rel, err := ShardKey(contentHash)
	if err != nil {
		return "", err
	}
	return filepath.Join(fs.Root, filepath.FromSlash(rel)), nil
}

// URIFor returns a portable cas://<hash> URI (store context is external).
func (fs *FileSystem) URIFor(contentHash string) (string, error) {
	h, err := NormalizeHash(contentHash)
	if err != nil {
		return "", err
	}
	return "cas://" + h, nil
}

// Put stores data under its content hash. Idempotent when the object exists.
func (fs *FileSystem) Put(data []byte) (string, error) {
	h := ContentHash(data)
	path, err := fs.PathFor(h)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		existing, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if ContentHash(existing) != h {
			return "", fmt.Errorf("%w: corrupt object at %s", ErrIntegrity, path)
		}
		return h, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+h+".*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	ok = true
	return h, nil
}

// Get loads bytes and verifies the content hash.
func (fs *FileSystem) Get(contentHash string) ([]byte, error) {
	h, err := NormalizeHash(contentHash)
	if err != nil {
		return nil, err
	}
	path, err := fs.PathFor(h)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, h)
		}
		return nil, err
	}
	actual := ContentHash(data)
	if actual != h {
		return nil, fmt.Errorf("%w: expected %s got %s at %s", ErrIntegrity, h, actual, path)
	}
	return data, nil
}

// Has reports whether an object path exists (does not full verify).
func (fs *FileSystem) Has(contentHash string) bool {
	h, err := NormalizeHash(contentHash)
	if err != nil {
		return false
	}
	path, err := fs.PathFor(h)
	if err != nil {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// ArtifactEntry is one row from a .tir artifacts-manifest.json.
type ArtifactEntry struct {
	LogicalPath string `json:"logical_path"`
	ContentHash string `json:"content_hash"`
	URI         string `json:"uri,omitempty"`
	Size        *int   `json:"size,omitempty"`
}

// Rehydrate loads bytes for each manifest entry from store.
// When requireAll is true, missing hashes return ErrNotFound.
func Rehydrate(store Store, entries []ArtifactEntry, requireAll bool) (map[string][]byte, error) {
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		h, err := NormalizeHash(e.ContentHash)
		if err != nil {
			return nil, err
		}
		if _, ok := out[h]; ok {
			continue
		}
		data, err := store.Get(h)
		if err != nil {
			if errors.Is(err, ErrNotFound) && !requireAll {
				continue
			}
			return nil, err
		}
		out[h] = data
	}
	return out, nil
}
