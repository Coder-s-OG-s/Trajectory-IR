package hosthook_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/hosthook"
)

type capture struct {
	mu     sync.Mutex
	events []emit.Event
}

func (c *capture) Emit(e emit.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return nil
}

// kinds lists "event kind" or "node.appended:NODE_KIND" in order.
func (c *capture) kinds() []string {
	out := []string{}
	for _, e := range c.events {
		if e.Kind == emit.KindNodeAppended {
			out = append(out, e.Kind+":"+e.Payload["kind"].(string))
		} else {
			out = append(out, e.Kind)
		}
	}
	return out
}

func (c *capture) reset() { c.events = nil }

func (c *capture) find(t *testing.T, kind, nodeKind string) emit.Event {
	t.Helper()
	for _, e := range c.events {
		if e.Kind == kind && (nodeKind == "" || e.Payload["kind"] == nodeKind) {
			return e
		}
	}
	t.Fatalf("no %s %s in %v", kind, nodeKind, c.kinds())
	return emit.Event{}
}

func consoleSession(t *testing.T) (hosthook.Session, *capture) {
	t.Helper()
	c := &capture{}
	s := newSession(t)
	s.Sink = c
	s.Runtime = "claude-code"
	return s, c
}

func TestSealEmitsNodesAndSeal(t *testing.T) {
	s, c := consoleSession(t)
	mustSeal(t, s, "echo", map[string]any{"msg": "hi"}, "t1")
	want := []string{"node.appended:PROJECT_CONTEXT", "node.appended:DECISION", "seal.created"}
	if got := c.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	seal := c.find(t, emit.KindSealCreated, "")
	decision := c.find(t, emit.KindNodeAppended, "DECISION")
	if seal.TrajectoryID != "claude-s1" || seal.TenantID != "local" || seal.Runtime != "claude-code" {
		t.Fatalf("seal envelope = %+v", seal)
	}
	if seal.Payload["node_id"] != decision.Payload["node_id"] || seal.Payload["step_n"] != 1 {
		t.Fatalf("seal payload = %v, decision = %v", seal.Payload, decision.Payload)
	}
	if h, _ := seal.Payload["content_hash"].(string); h == "" {
		t.Fatalf("seal has no content_hash: %v", seal.Payload)
	}
	if !reflect.DeepEqual(seal.Payload["tool_names"], []string{"echo"}) {
		t.Fatalf("tool_names = %v", seal.Payload["tool_names"])
	}
}

func TestResultEmitsToolNodes(t *testing.T) {
	s, c := consoleSession(t)
	mustSeal(t, s, "deploy", map[string]any{"env": "prod"}, "t1")
	c.reset()
	if _, err := s.RecordToolResult("t1", "ok"); err != nil {
		t.Fatal(err)
	}
	want := []string{"node.appended:TOOL_CALL", "node.appended:TOOL_RESULT", "node.appended:COMMIT_STEP"}
	if got := c.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	call := c.find(t, emit.KindNodeAppended, "TOOL_CALL").Payload
	if call["tool"] != "deploy" || call["effect_class"] != "NON_IDEMPOTENT_WRITE" || call["step_n"] != 1 || call["seq"] != 2 {
		t.Fatalf("TOOL_CALL event = %v", call)
	}
	if k, _ := call["idempotency_key"].(string); k == "" {
		t.Fatalf("TOOL_CALL event has no idempotency_key: %v", call)
	}
}

func TestFailureEmitsAbort(t *testing.T) {
	s, c := consoleSession(t)
	mustSeal(t, s, "deploy", nil, "t1")
	c.reset()
	if _, err := s.RecordToolFailure("t1", "boom"); err != nil {
		t.Fatal(err)
	}
	want := []string{"node.appended:TOOL_CALL", "node.appended:ABORT", "node.appended:COMMIT_STEP"}
	if got := c.kinds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func TestReplayEmitsNothing(t *testing.T) {
	s, c := consoleSession(t)
	mustSeal(t, s, "deploy", nil, "t1")
	c.reset()
	mustSeal(t, s, "deploy", nil, "t1")
	if len(c.events) != 0 {
		t.Fatalf("replay emitted %v", c.kinds())
	}
}

func TestSinkErrorDoesNotFailHook(t *testing.T) {
	s := newSession(t)
	s.Sink = emit.SinkFunc(func(emit.Event) error { return errors.New("disk full") })
	mustSeal(t, s, "echo", nil, "t1")
	if _, err := s.RecordToolResult("t1", "ok"); err != nil {
		t.Fatalf("sink error reached the hook: %v", err)
	}
}

func TestDefaultRuntimeLabel(t *testing.T) {
	s, c := consoleSession(t)
	s.Runtime = ""
	mustSeal(t, s, "echo", nil, "t1")
	if got := c.events[0].Runtime; got != "trajir-hook" {
		t.Fatalf("runtime = %q", got)
	}
}
