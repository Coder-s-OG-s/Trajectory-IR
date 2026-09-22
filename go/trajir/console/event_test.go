package console

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func sampleEvent(kind string) Event {
	return Event{
		SchemaVersion: SchemaVersion,
		ID:            "evt-1",
		TS:            time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Kind:          kind,
		Source:        "go",
		TrajectoryID:  "t-demo",
		Payload:       json.RawMessage(`{}`),
	}
}

func TestValidateRejectsBadEnvelope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mut  func(*Event)
	}{
		{"schema", func(e *Event) { e.SchemaVersion = "nope" }},
		{"id", func(e *Event) { e.ID = "" }},
		{"trajectory", func(e *Event) { e.TrajectoryID = "" }},
		{"path traj", func(e *Event) { e.TrajectoryID = "../x" }},
		{"kind", func(e *Event) { e.Kind = "" }},
		{"source", func(e *Event) { e.Source = "java" }},
		{"ts", func(e *Event) { e.TS = "yesterday" }},
		{"payload array", func(e *Event) { e.Payload = json.RawMessage(`[]`) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := sampleEvent(KindNodeAppended)
			tc.mut(&e)
			if err := e.Validate(); !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("Validate() err=%v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestParseEventRoundTrip(t *testing.T) {
	t.Parallel()
	e := sampleEvent(KindSealCreated)
	e.Payload = json.RawMessage(`{"node_id":"n1","step_n":1,"content_hash":"abc"}`)
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindSealCreated || got.TrajectoryID != "t-demo" {
		t.Fatalf("got %+v", got)
	}
}

func TestEstimatedTokens(t *testing.T) {
	t.Parallel()
	if EstimatedTokens(0) != 0 {
		t.Fatal()
	}
	if EstimatedTokens(1) != 1 {
		t.Fatal()
	}
	if EstimatedTokens(4) != 1 {
		t.Fatal()
	}
	if EstimatedTokens(5) != 2 {
		t.Fatal()
	}
}
