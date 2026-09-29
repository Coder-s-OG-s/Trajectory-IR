package console

import (
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

func TestFileSinkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sink := &emit.FileSink{Dir: dir}
	emit.SafeEmit(sink, emit.Event{
		Kind:         emit.KindSealCreated,
		Source:       "go",
		TrajectoryID: "round-trip",
		TenantID:     "demo",
		Runtime:      "go",
		Payload: map[string]any{
			"node_id":      "n1",
			"step_n":       1,
			"content_hash": "abc",
		},
	})
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	events, err := st.ReadEvents("round-trip")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != KindSealCreated {
		t.Fatalf("%+v", events)
	}
	if Summarize("round-trip", events).SealCreatedCount != 1 {
		t.Fatal("seal count")
	}
}
