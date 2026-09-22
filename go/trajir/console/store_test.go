package console

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAppendListRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}

	e1 := sampleEvent(KindNodeAppended)
	e1.ID = "a"
	e1.Payload = json.RawMessage(`{"node_id":"n1","kind":"DECISION"}`)
	e2 := sampleEvent(KindSealCreated)
	e2.ID = "b"
	e2.TS = time.Date(2026, 9, 22, 12, 1, 0, 0, time.UTC).Format(time.RFC3339)
	e2.Payload = json.RawMessage(`{"node_id":"n1","step_n":1,"content_hash":"h"}`)

	if err := st.Append(e1); err != nil {
		t.Fatal(err)
	}
	if err := st.Append(e2); err != nil {
		t.Fatal(err)
	}

	ids, err := st.ListTrajectories()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "t-demo" {
		t.Fatalf("ids=%v", ids)
	}

	events, err := st.ReadEvents("t-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("len=%d", len(events))
	}
	if events[0].ID != "a" || events[1].ID != "b" {
		t.Fatalf("order %+v", events)
	}

	if _, err := st.ReadEvents("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}

	path := filepath.Join(root, "trajectories", "t-demo.ndjson")
	if path == "" {
		t.Fatal("expected path")
	}
}

func TestStoreRejectsInvalidAppend(t *testing.T) {
	t.Parallel()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e := sampleEvent(KindNodeAppended)
	e.TrajectoryID = "a/b"
	if err := st.Append(e); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("err=%v", err)
	}
}
