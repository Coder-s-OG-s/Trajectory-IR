// Package effects ports Trajectory IR tool effect classes and MCP mapping.
// Rules match pkg/trajectory_ir/effects/classify.py (fail closed).
package effects

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

	if hasReadOnly && readOnly && !(hasDestructive && destructive) {
		return READ_ONLY
	}
	if hasReadOnly && !readOnly && hasIdempotent && idempotent && hasDestructive && !destructive {
		return IDEMPOTENT_WRITE
	}
	return NON_IDEMPOTENT_WRITE
}

func asBool(v any) (value bool, ok bool) {
	b, ok := v.(bool)
	return b, ok
}
