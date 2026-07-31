package effects_test

import (
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
)

func TestRequiresBlockAndGate(t *testing.T) {
	if !effects.RequiresBlockAndGate(effects.NON_IDEMPOTENT_WRITE) {
		t.Fatal("NON_IDEMPOTENT_WRITE must be gated (R02)")
	}
	for _, e := range []effects.EffectClass{
		effects.PURE,
		effects.READ_ONLY,
		effects.IDEMPOTENT_WRITE,
		effects.AGENT_SPAWN,
		effects.SENSITIVE,
	} {
		if effects.RequiresBlockAndGate(e) {
			t.Fatalf("%s must not be gated (R03 matrix)", e)
		}
	}
}

func TestMissingAnnotationsFailClosed(t *testing.T) {
	if got := effects.ClassifyFromMCP(map[string]any{}); got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("got %s, want NON_IDEMPOTENT_WRITE", got)
	}
	if got := effects.ClassifyFromMCP(nil); got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("nil map: got %s", got)
	}
}

func TestAmbiguousAnnotationsFailClosed(t *testing.T) {
	got := effects.ClassifyFromMCP(map[string]any{"readOnlyHint": false})
	if got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("got %s, want NON_IDEMPOTENT_WRITE", got)
	}
}

func TestReadOnlyHint(t *testing.T) {
	got := effects.ClassifyFromMCP(map[string]any{"readOnlyHint": true})
	if got != effects.READ_ONLY {
		t.Fatalf("got %s, want READ_ONLY", got)
	}
}

func TestExplicitIdempotentWrite(t *testing.T) {
	got := effects.ClassifyFromMCP(map[string]any{
		"readOnlyHint":    false,
		"idempotentHint":  true,
		"destructiveHint": false,
	})
	if got != effects.IDEMPOTENT_WRITE {
		t.Fatalf("got %s, want IDEMPOTENT_WRITE", got)
	}
}

func TestDestructiveFailsClosedEvenIfIdempotent(t *testing.T) {
	got := effects.ClassifyFromMCP(map[string]any{
		"readOnlyHint":    false,
		"idempotentHint":  true,
		"destructiveHint": true,
	})
	if got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("got %s, want NON_IDEMPOTENT_WRITE", got)
	}
}

func TestContradictoryReadOnlyAndDestructive(t *testing.T) {
	got := effects.ClassifyFromMCP(map[string]any{
		"readOnlyHint":    true,
		"destructiveHint": true,
	})
	if got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("got %s, want NON_IDEMPOTENT_WRITE", got)
	}
}

func TestStringValuesMatchPython(t *testing.T) {
	want := map[effects.EffectClass]string{
		effects.PURE:                 "PURE",
		effects.READ_ONLY:            "READ_ONLY",
		effects.IDEMPOTENT_WRITE:     "IDEMPOTENT_WRITE",
		effects.NON_IDEMPOTENT_WRITE: "NON_IDEMPOTENT_WRITE",
		effects.AGENT_SPAWN:          "AGENT_SPAWN",
		effects.SENSITIVE:            "SENSITIVE",
	}
	for c, s := range want {
		if string(c) != s {
			t.Fatalf("%v string is %q, want %q", c, string(c), s)
		}
	}
}
