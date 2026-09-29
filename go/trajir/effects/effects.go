// Package effects ports Trajectory IR tool effect classes and MCP mapping.
// Rules match pkg/trajectory_ir/effects/classify.py (fail closed).
package effects

import "strings"

// EffectClass is how dangerous a tool is for resume and gate policy.
type EffectClass string

const (
	PURE                 EffectClass = "PURE"
	READ_ONLY            EffectClass = "READ_ONLY"
	IDEMPOTENT_WRITE     EffectClass = "IDEMPOTENT_WRITE"
	NON_IDEMPOTENT_WRITE EffectClass = "NON_IDEMPOTENT_WRITE"
	AGENT_SPAWN          EffectClass = "AGENT_SPAWN"
	SENSITIVE            EffectClass = "SENSITIVE"
)

// RequiresBlockAndGate reports whether resume must gate this effect (R02).
// Only NON_IDEMPOTENT_WRITE is gated. PURE (R03) and other classes may
// re-execute on resume without BlockedNeedsGate.
func RequiresBlockAndGate(e EffectClass) bool {
	return e == NON_IDEMPOTENT_WRITE
}

func IsForbiddenInSandbox(e EffectClass) bool {
	return e == NON_IDEMPOTENT_WRITE || e == AGENT_SPAWN || e == SENSITIVE
}

// OpenWorldPrimitives are non-exhaustive names for arbitrary-execution tools.
// The same primitive can read, write, spawn, or destroy depending on args.
// Static MCP hints on the tool definition cannot classify them. This is not
// an AST analyzer.
var OpenWorldPrimitives = map[string]struct{}{
	"bash":               {},
	"shell":              {},
	"sh":                 {},
	"zsh":                {},
	"terminal":           {},
	"execute":            {},
	"python":             {},
	"python_interpreter": {},
	"code_interpreter":   {},
	"sql":                {},
	"sql_query":          {},
	"execute_sql":        {},
	"browser":            {},
	"browser_action":     {},
	"computer":           {},
}

// IsOpenWorldPrimitive reports whether name is a known arbitrary-execution tool.
func IsOpenWorldPrimitive(name string) bool {
	_, ok := OpenWorldPrimitives[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// ClassifyTool classifies a named tool. Open-world primitives fail closed
// even if MCP hints claim read-only. Operators override by setting
// Tool.Effect directly, not by lying on the hint bits.
func ClassifyTool(name string, annotations map[string]any) EffectClass {
	if IsOpenWorldPrimitive(name) {
		return NON_IDEMPOTENT_WRITE
	}
	return ClassifyFromMCP(annotations)
}

// ClassifyFromMCP maps MCP tool annotations to an EffectClass.
// Missing or ambiguous input becomes NON_IDEMPOTENT_WRITE.
//
// Python treats annotations.get("x") is True / is False strictly, so only
// boolean true/false count. Other JSON types fall through to fail closed.
func ClassifyFromMCP(annotations map[string]any) EffectClass {
	if annotations == nil {
		return NON_IDEMPOTENT_WRITE
	}

	readOnly, hasReadOnly := asBool(annotations["readOnlyHint"])
	destructive, hasDestructive := asBool(annotations["destructiveHint"])
	idempotent, hasIdempotent := asBool(annotations["idempotentHint"])
	openWorld, hasOpenWorld := asBool(annotations["openWorldHint"])

	if hasReadOnly && readOnly && !(hasDestructive && destructive) {
		return READ_ONLY
	}
	// openWorldHint=true without a clear safe profile stays fail-closed below.
	if hasReadOnly && !readOnly && hasIdempotent && idempotent && hasDestructive && !destructive && !(hasOpenWorld && openWorld) {
		return IDEMPOTENT_WRITE
	}
	return NON_IDEMPOTENT_WRITE
}

func asBool(v any) (value bool, ok bool) {
	b, ok := v.(bool)
	return b, ok
}
