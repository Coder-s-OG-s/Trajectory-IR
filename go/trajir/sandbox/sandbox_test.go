package sandbox_test

import (
	"errors"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
)

func TestSandboxRejectsNonIdempotent(t *testing.T) {
	err := sandbox.AssertToolAllowed(sandbox.ModeSandbox, "deploy", effects.NON_IDEMPOTENT_WRITE)
	var f *sandbox.Forbidden
	if !errors.As(err, &f) {
		t.Fatalf("want Forbidden, got %v", err)
	}
}

func TestSandboxAllowsPure(t *testing.T) {
	if err := sandbox.AssertToolAllowed(sandbox.ModeSandbox, "compute", effects.PURE); err != nil {
		t.Fatal(err)
	}
}

func TestLiveAllowsNonIdempotent(t *testing.T) {
	if err := sandbox.AssertToolAllowed(sandbox.ModeLive, "deploy", effects.NON_IDEMPOTENT_WRITE); err != nil {
		t.Fatal(err)
	}
}
