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

func TestSandboxRejectsAgentSpawn(t *testing.T) {
	err := sandbox.AssertToolAllowed(sandbox.ModeSandbox, "spawn_worker", effects.AGENT_SPAWN)
	var f *sandbox.Forbidden
	if !errors.As(err, &f) {
		t.Fatalf("want Forbidden, got %v", err)
	}
}

func TestSandboxRejectsSensitive(t *testing.T) {
	err := sandbox.AssertToolAllowed(sandbox.ModeSandbox, "read_api_key", effects.SENSITIVE)
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

func TestSandboxAllowsReadOnly(t *testing.T) {
	if err := sandbox.AssertToolAllowed(sandbox.ModeSandbox, "read_config", effects.READ_ONLY); err != nil {
		t.Fatal(err)
	}
}

func TestLiveAllowsNonIdempotent(t *testing.T) {
	if err := sandbox.AssertToolAllowed(sandbox.ModeLive, "deploy", effects.NON_IDEMPOTENT_WRITE); err != nil {
		t.Fatal(err)
	}
}

func TestLiveAllowsAgentSpawn(t *testing.T) {
	if err := sandbox.AssertToolAllowed(sandbox.ModeLive, "spawn_worker", effects.AGENT_SPAWN); err != nil {
		t.Fatal(err)
	}
}

func TestLiveAllowsSensitive(t *testing.T) {
	if err := sandbox.AssertToolAllowed(sandbox.ModeLive, "read_api_key", effects.SENSITIVE); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeMode(t *testing.T) {
	cases := []struct {
		in      string
		want    sandbox.Mode
		wantErr bool
	}{
		{"", sandbox.ModeLive, false},
		{"live", sandbox.ModeLive, false},
		{"  LIVE  ", sandbox.ModeLive, false},
		{"sandbox", sandbox.ModeSandbox, false},
		{"SANDBOX", sandbox.ModeSandbox, false},
		{"bogus", "", true},
	}
	for _, c := range cases {
		got, err := sandbox.NormalizeMode(c.in)
		if c.wantErr {
			if err == nil {
				t.Fatalf("NormalizeMode(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeMode(%q): unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("NormalizeMode(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestForbiddenErrorText(t *testing.T) {
	e := &sandbox.Forbidden{ToolName: "deploy", Effect: effects.NON_IDEMPOTENT_WRITE}
	msg := e.Error()
	if msg == "" {
		t.Fatal("empty error text")
	}
}

