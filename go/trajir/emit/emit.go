// Package emit is a small console sink for hosts.
//
// The default is off. A nil Sink does nothing. Sink failures are logged and
// never returned to the durable path. This package does not import the
// console UI.
package emit

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const SchemaVersion = "console-events-v1"

const (
	KindNodeAppended    = "node.appended"
	KindSealCreated     = "seal.created"
	KindExportCompleted = "export.completed"
	KindImportCompleted = "import.completed"
)

// Event is one console-events-v1 object.
type Event struct {
	SchemaVersion string         `json:"schema_version"`
	ID            string         `json:"id"`
	TS            string         `json:"ts"`
	Kind          string         `json:"kind"`
	Source        string         `json:"source"`
	TrajectoryID  string         `json:"trajectory_id"`
	TenantID      string         `json:"tenant_id,omitempty"`
	Runtime       string         `json:"runtime,omitempty"`
	Payload       map[string]any `json:"payload"`
}

// Sink accepts one event. Implementations must be safe to call after a
// successful node append or package write.
type Sink interface {
	Emit(Event) error
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(Event) error

func (f SinkFunc) Emit(e Event) error {
	if f == nil {
		return nil
	}
	return f(e)
}

// FileSink appends NDJSON under Dir/trajectories/<id>.ndjson, the same layout
// the console store reads.
type FileSink struct {
	Dir string
	mu  sync.Mutex
}

func (f *FileSink) Emit(e Event) error {
	if f == nil || strings.TrimSpace(f.Dir) == "" {
		return errors.New("emit: file sink data dir required")
	}
	e = normalize(e)
	if err := validTrajectoryID(e.TrajectoryID); err != nil {
		return err
	}
	dir := filepath.Join(f.Dir, "trajectories")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, e.TrajectoryID+".ndjson")
	f.mu.Lock()
	defer f.mu.Unlock()
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(append(line, '\n'))
	return err
}

// HTTPSink POSTs one event to BaseURL/v1/events.
type HTTPSink struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (h *HTTPSink) Emit(e Event) error {
	if h == nil || strings.TrimSpace(h.BaseURL) == "" {
		return errors.New("emit: http sink url required")
	}
	e = normalize(e)
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	url := strings.TrimRight(h.BaseURL, "/") + "/v1/events"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("emit: console http status %d", resp.StatusCode)
	}
	return nil
}

// FromEnv returns a sink when TRAJIR_CONSOLE_SINK is file or http.
// Any other value, including empty, returns nil.
func FromEnv() Sink {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TRAJIR_CONSOLE_SINK"))) {
	case "file":
		return &FileSink{Dir: os.Getenv("TRAJIR_CONSOLE_DATA")}
	case "http":
		url := strings.TrimSpace(os.Getenv("TRAJIR_CONSOLE_URL"))
		if url == "" {
			url = "http://127.0.0.1:8787"
		}
		return &HTTPSink{BaseURL: url, Token: os.Getenv("TRAJIR_CONSOLE_TOKEN")}
	default:
		return nil
	}
}

// SafeEmit fills the envelope and logs sink errors. It does not return them.
// A panic inside the sink is recovered.
func SafeEmit(s Sink, e Event) {
	if s == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("console sink: %v", r)
		}
	}()
	if err := s.Emit(normalize(e)); err != nil {
		log.Printf("console sink: %v", err)
	}
}

// PackageFact is the payload for export.completed or import.completed.
type PackageFact struct {
	Kind         string
	TrajectoryID string
	TenantID     string
	Path         string
	Mode         string
	Redacted     *bool
	Bytes        int64
	MemberCount  int
	NodeCount    int
	OK           *bool
	Error        string
	Source       string
	Runtime      string
}

// NotePackage emits one package event. Nil sink is a no-op.
func NotePackage(s Sink, f PackageFact) {
	payload := map[string]any{
		"path":         f.Path,
		"mode":         f.Mode,
		"bytes":        f.Bytes,
		"member_count": f.MemberCount,
		"node_count":   f.NodeCount,
	}
	if f.Redacted != nil {
		payload["redacted"] = *f.Redacted
	}
	if f.OK != nil {
		if f.Kind == KindImportCompleted {
			payload["verify_ok"] = *f.OK
		} else {
			payload["ok"] = *f.OK
		}
	}
	if f.Error != "" {
		payload["error"] = f.Error
	}
	source := f.Source
	if source == "" {
		source = "go"
	}
	SafeEmit(s, Event{
		Kind:         f.Kind,
		Source:       source,
		Runtime:      f.Runtime,
		TrajectoryID: f.TrajectoryID,
		TenantID:     f.TenantID,
		Payload:      payload,
	})
}

func normalize(e Event) Event {
	if e.SchemaVersion == "" {
		e.SchemaVersion = SchemaVersion
	}
	if e.ID == "" {
		e.ID = newID()
	}
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if e.Source == "" {
		e.Source = "go"
	}
	if e.Payload == nil {
		e.Payload = map[string]any{}
	}
	return e
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

func validTrajectoryID(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("emit: trajectory_id required")
	}
	if strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") || len(id) > 200 {
		return errors.New("emit: bad trajectory_id")
	}
	return nil
}
