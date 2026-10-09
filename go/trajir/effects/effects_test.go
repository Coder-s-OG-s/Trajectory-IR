package effects_test

import (
	"errors"
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

func TestIsForbiddenInSandbox(t *testing.T) {
	forbidden := []effects.EffectClass{
		effects.NON_IDEMPOTENT_WRITE,
		effects.AGENT_SPAWN,
		effects.SENSITIVE,
	}
	for _, e := range forbidden {
		if !effects.IsForbiddenInSandbox(e) {
			t.Fatalf("%s must be forbidden in sandbox", e)
		}
	}
	allowed := []effects.EffectClass{
		effects.PURE,
		effects.READ_ONLY,
		effects.IDEMPOTENT_WRITE,
	}
	for _, e := range allowed {
		if effects.IsForbiddenInSandbox(e) {
			t.Fatalf("%s must be allowed in sandbox", e)
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

func TestOpenWorldFailsClosedEvenIfIdempotent(t *testing.T) {
	got := effects.ClassifyFromMCP(map[string]any{
		"readOnlyHint":    false,
		"idempotentHint":  true,
		"destructiveHint": false,
		"openWorldHint":   true,
	})
	if got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("got %s, want NON_IDEMPOTENT_WRITE", got)
	}
}

func TestOpenWorldFalseStillAllowsIdempotentWrite(t *testing.T) {
	got := effects.ClassifyFromMCP(map[string]any{
		"readOnlyHint":    false,
		"idempotentHint":  true,
		"destructiveHint": false,
		"openWorldHint":   false,
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

func TestClassifyToolFailClosesBashEvenIfReadOnlyHint(t *testing.T) {
	readOnly := map[string]any{"readOnlyHint": true}
	if got := effects.ClassifyFromMCP(readOnly); got != effects.READ_ONLY {
		t.Fatalf("MCP mapper: got %s", got)
	}
	for _, name := range []string{"bash", "BASH", "python_interpreter", "sql_query"} {
		if got := effects.ClassifyTool(name, readOnly); got != effects.NON_IDEMPOTENT_WRITE {
			t.Fatalf("%s: got %s, want NON_IDEMPOTENT_WRITE", name, got)
		}
	}
}

func TestClassifyToolLeavesOrdinaryNamesToMCP(t *testing.T) {
	if got := effects.ClassifyTool("echo", map[string]any{"readOnlyHint": true}); got != effects.READ_ONLY {
		t.Fatalf("echo: got %s", got)
	}
	if got := effects.ClassifyTool("charge_card", map[string]any{}); got != effects.NON_IDEMPOTENT_WRITE {
		t.Fatalf("charge_card: got %s", got)
	}
}

func TestAssertOpenWorldEffect(t *testing.T) {
	err := effects.AssertOpenWorldEffect("bash", effects.READ_ONLY, false)
	var ow *effects.OpenWorldOverrideRequired
	if !errors.As(err, &ow) {
		t.Fatalf("err=%v want OpenWorldOverrideRequired", err)
	}
	if err := effects.AssertOpenWorldEffect("python", effects.PURE, false); err == nil {
		t.Fatal("python PURE must be refused")
	}
	if err := effects.AssertOpenWorldEffect("sql", effects.IDEMPOTENT_WRITE, false); err == nil {
		t.Fatal("sql IDEMPOTENT_WRITE must be refused")
	}
	if err := effects.AssertOpenWorldEffect("bash", effects.NON_IDEMPOTENT_WRITE, false); err != nil {
		t.Fatal(err)
	}
	if err := effects.AssertOpenWorldEffect("bash", effects.AGENT_SPAWN, false); err != nil {
		t.Fatal(err)
	}
	if err := effects.AssertOpenWorldEffect("echo", effects.READ_ONLY, false); err != nil {
		t.Fatal(err)
	}
	if err := effects.AssertOpenWorldEffect("bash", effects.READ_ONLY, true); err != nil {
		t.Fatal(err)
	}
}

func TestIsOpenWorldPrimitive(t *testing.T) {
	if !effects.IsOpenWorldPrimitive("bash") || !effects.IsOpenWorldPrimitive(" Shell ") {
		t.Fatal("expected open-world names")
	}
	if effects.IsOpenWorldPrimitive("echo") {
		t.Fatal("echo is not an open-world primitive")
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
