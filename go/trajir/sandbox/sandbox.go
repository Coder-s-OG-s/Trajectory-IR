// Package sandbox implements live vs sandbox run modes (R06).
package sandbox

import (
	"fmt"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
)

// Mode is the trajectory execution mode.
type Mode string

const (
	ModeLive    Mode = "live"
	ModeSandbox Mode = "sandbox"
)

// Forbidden is raised when sandbox rejects a tool effect.
type Forbidden struct {
	ToolName string
	Effect   effects.EffectClass
}

func (e *Forbidden) Error() string {
	return fmt.Sprintf(
		"SANDBOX_REJECTS_NON_IDEMPOTENT_WRITE: tool %q effect=%s is forbidden in sandbox mode",
		e.ToolName,
		e.Effect,
	)
}

// NormalizeMode maps string/empty to ModeLive or ModeSandbox.
func NormalizeMode(mode string) (Mode, error) {
	m := strings.TrimSpace(strings.ToLower(mode))
	if m == "" || m == "live" {
		return ModeLive, nil
	}
	if m == "sandbox" {
		return ModeSandbox, nil
	}
	return "", fmt.Errorf("unsupported run mode %q; use live or sandbox", mode)
}

// AssertToolAllowed raises Forbidden in sandbox for gated effects.
func AssertToolAllowed(mode Mode, toolName string, effect effects.EffectClass) error {
	if mode == ModeSandbox && effects.RequiresBlockAndGate(effect) {
		return &Forbidden{ToolName: toolName, Effect: effect}
	}
	return nil
}
