// Package console implements append-only ingest and read APIs for Trajectory
// console events (docs/CONSOLE_EVENTS.md, schema console-events-v1).
//
// This package must not import UI frameworks. Hosts emit events; the console
// process stores and aggregates them.
package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = "console-events-v1"

// Known event kinds for console-events-v1.
const (
	KindNodeAppended     = "node.appended"
	KindSealCreated      = "seal.created"
	KindSealVerified     = "seal.verified"
	KindContextProjected = "context.projected"
	KindRedactionApplied = "redaction.applied"
	KindExportStarted    = "export.started"
	KindExportCompleted  = "export.completed"
	KindImportCompleted  = "import.completed"
)

var knownKinds = map[string]struct{}{
	KindNodeAppended:     {},
	KindSealCreated:      {},
	KindSealVerified:     {},
	KindContextProjected: {},
	KindRedactionApplied: {},
	KindExportStarted:    {},
	KindExportCompleted:  {},
	KindImportCompleted:  {},
}

var knownSources = map[string]struct{}{
	"go":      {},
	"python":  {},
	"cli":     {},
	"derived": {},
}

var (
	ErrInvalidEvent = errors.New("console: invalid event")
	ErrNotFound     = errors.New("console: trajectory not found")
)

// Event is one console observation (one NDJSON line).
type Event struct {
	SchemaVersion string          `json:"schema_version"`
	ID            string          `json:"id"`
	TS            string          `json:"ts"`
	Kind          string          `json:"kind"`
	Source        string          `json:"source"`
	TrajectoryID  string          `json:"trajectory_id"`
	TenantID      string          `json:"tenant_id,omitempty"`
	Runtime       string          `json:"runtime,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

// Validate checks required envelope fields. Unknown kinds are allowed when the
// envelope is otherwise valid (forward compatible). Malformed required fields
// fail closed.
func (e *Event) Validate() error {
	if e == nil {
		return fmt.Errorf("%w: nil event", ErrInvalidEvent)
	}
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema_version %q (want %s)", ErrInvalidEvent, e.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("%w: id required", ErrInvalidEvent)
	}
	if strings.TrimSpace(e.TrajectoryID) == "" {
		return fmt.Errorf("%w: trajectory_id required", ErrInvalidEvent)
	}
	if err := validateTrajectoryID(e.TrajectoryID); err != nil {
		return err
	}
	if strings.TrimSpace(e.Kind) == "" {
		return fmt.Errorf("%w: kind required", ErrInvalidEvent)
	}
	if _, ok := knownSources[e.Source]; !ok {
		return fmt.Errorf("%w: source %q", ErrInvalidEvent, e.Source)
	}
	if _, err := time.Parse(time.RFC3339, e.TS); err != nil {
		return fmt.Errorf("%w: ts must be RFC3339: %v", ErrInvalidEvent, err)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: payload required", ErrInvalidEvent)
	}
	if !json.Valid(e.Payload) {
		return fmt.Errorf("%w: payload is not valid JSON", ErrInvalidEvent)
	}
	var probe any
	if err := json.Unmarshal(e.Payload, &probe); err != nil {
		return fmt.Errorf("%w: payload: %v", ErrInvalidEvent, err)
	}
	if _, ok := probe.(map[string]any); !ok {
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalidEvent)
	}
	return nil
}

func validateTrajectoryID(id string) error {
	if strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("%w: trajectory_id must not contain path separators", ErrInvalidEvent)
	}
	if len(id) > 200 {
		return fmt.Errorf("%w: trajectory_id too long", ErrInvalidEvent)
	}
	return nil
}

// ParseEvent unmarshals and validates one event object.
func ParseEvent(raw []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	if err := e.Validate(); err != nil {
		return Event{}, err
	}
	return e, nil
}

// EstimatedTokens is the v1 console estimator: ceil(charLen / 4).
func EstimatedTokens(charLen int) int {
	if charLen <= 0 {
		return 0
	}
	return (charLen + 3) / 4
}

// IsKnownKind reports whether kind is defined in console-events-v1.
func IsKnownKind(kind string) bool {
	_, ok := knownKinds[kind]
	return ok
}
