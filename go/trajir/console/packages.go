package console

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

const packagesDir = emit.PackagesDir

// PackageInfo is one locally stored .tir copy.
type PackageInfo struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Rel   string `json:"rel"`
}

func withConsolePath(e Event, rel string) Event {
	var m map[string]any
	if err := json.Unmarshal(e.Payload, &m); err != nil {
		return e
	}
	m["console_path"] = rel
	b, err := json.Marshal(m)
	if err != nil {
		return e
	}
	e.Payload = b
	return e
}

func (s *Store) maybeStageExport(e Event) Event {
	if e.Kind != KindExportCompleted {
		return e
	}
	src, ok := payloadString(e.Payload, "path")
	if !ok || strings.TrimSpace(src) == "" {
		return e
	}
	rel, err := emit.StagePackage(s.root, e.TrajectoryID, src, e.ID)
	if err != nil || rel == "" {
		return e
	}
	return withConsolePath(e, rel)
}

// ListPackages returns staged .tir files for a trajectory, sorted by name.
func (s *Store) ListPackages(trajectoryID string) ([]PackageInfo, error) {
	if err := validateTrajectoryID(trajectoryID); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, packagesDir, trajectoryID)
	s.mu.RLock()
	entries, err := os.ReadDir(dir)
	s.mu.RUnlock()
	if err != nil {
		if os.IsNotExist(err) {
			return []PackageInfo{}, nil
		}
		return nil, fmt.Errorf("console: list packages: %w", err)
	}
	out := make([]PackageInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, err := emit.SafeTirName(e.Name())
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, PackageInfo{
			Name:  name,
			Bytes: info.Size(),
			Rel:   emit.RelConsolePath(trajectoryID, name),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// PackagePath resolves a staged package. The file must already exist.
func (s *Store) PackagePath(trajectoryID, name string) (string, error) {
	if err := validateTrajectoryID(trajectoryID); err != nil {
		return "", err
	}
	safe, err := emit.SafeTirName(name)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	abs := filepath.Join(s.root, packagesDir, trajectoryID, safe)
	if !underRoot(s.root, abs) {
		return "", fmt.Errorf("%w: package path escapes data dir", ErrInvalidEvent)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, safe)
		}
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: not a file", ErrNotFound)
	}
	return abs, nil
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
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}
