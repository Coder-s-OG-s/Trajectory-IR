package console

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	envDataDir       = "TRAJIR_CONSOLE_DATA"
	trajectoriesDir  = "trajectories"
	defaultFilePerm  = 0o644
	defaultDirPerm   = 0o755
)

// Store is an append-only NDJSON event store keyed by trajectory id.
type Store struct {
	root string
	mu   sync.Mutex
}

// OpenStore uses TRAJIR_CONSOLE_DATA when root is empty.
func OpenStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		root = strings.TrimSpace(os.Getenv(envDataDir))
	}
	if root == "" {
		return nil, fmt.Errorf("console: data dir required (set %s or pass root)", envDataDir)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("console: resolve data dir: %w", err)
	}
	trajDir := filepath.Join(abs, trajectoriesDir)
	if err := os.MkdirAll(trajDir, defaultDirPerm); err != nil {
		return nil, fmt.Errorf("console: create data dir: %w", err)
	}
	return &Store{root: abs}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) pathFor(trajectoryID string) (string, error) {
	if err := validateTrajectoryID(trajectoryID); err != nil {
		return "", err
	}
	return filepath.Join(s.root, trajectoriesDir, trajectoryID+".ndjson"), nil
}

// Append validates and appends one event. Creates the trajectory file if needed.
func (s *Store) Append(e Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("console: marshal event: %w", err)
	}
	path, err := s.pathFor(e.TrajectoryID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, defaultFilePerm)
	if err != nil {
		return fmt.Errorf("console: open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("console: write: %w", err)
	}
	return nil
}

// ListTrajectories returns trajectory ids that have at least one event, sorted.
func (s *Store) ListTrajectories() ([]string, error) {
	dir := filepath.Join(s.root, trajectoriesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("console: list: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".ndjson") {
			continue
		}
		id := strings.TrimSuffix(name, ".ndjson")
		if err := validateTrajectoryID(id); err != nil {
			continue
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// ReadEvents returns events for a trajectory in file order (append order).
func (s *Store) ReadEvents(trajectoryID string) ([]Event, error) {
	path, err := s.pathFor(trajectoryID)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, trajectoryID)
		}
		return nil, fmt.Errorf("console: read %s: %w", path, err)
	}
	return parseNDJSON(raw)
}

func parseNDJSON(raw []byte) ([]Event, error) {
	sc := bufio.NewScanner(bytes.NewReader(raw))
	// Events can be large; allow up to 4 MiB per line.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4<<20)

	var out []Event
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		e, err := ParseEvent(line)
		if err != nil {
			return nil, fmt.Errorf("console: line %d: %w", lineNo, err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("console: scan: %w", err)
	}
	return out, nil
}
