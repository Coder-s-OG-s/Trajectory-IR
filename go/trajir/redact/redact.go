// Package redact implements shared secret/thought scrubbing for export and projection (R08).
package redact

import (
	"regexp"
)

var (
	secretKeyRE = regexp.MustCompile(`(?i)(password|passwd|pass[_-]?phrase|secret|token|api[_-]?key|access[_-]?key|authorization|auth|credential|private[_-]?key)`)
	secretValRE = regexp.MustCompile(
		`AKIA[0-9A-Z]{16}` +
			`|[Bb]earer\s+[A-Za-z0-9\-_.=]{10,}` +
			`|gh[pousr]_[A-Za-z0-9]{20,}` +
			`|sk-[A-Za-z0-9]{20,}` +
			`|xox[baprs]-[A-Za-z0-9-]{10,}` +
			`|-----BEGIN[ A-Z]*PRIVATE KEY-----` +
			`|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`,
	)
)

const Redacted = "[REDACTED]"

// RedactValue scrubs a value by key name or secret-shaped string.
func RedactValue(key string, value any) any {
	if secretKeyRE.MatchString(key) {
		return Redacted
	}
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, child := range v {
			out[k] = RedactValue(k, child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = RedactValue(key, child)
		}
		return out
	case string:
		if secretValRE.MatchString(v) {
			return Redacted
		}
		return v
	default:
		return value
	}
}

// RedactPayload scrubs a node payload for projection/export.
func RedactPayload(kind string, payload any) map[string]any {
	if kind == "THOUGHT" {
		return map[string]any{"redacted": true}
	}
	if m, ok := payload.(map[string]any); ok {
		out := make(map[string]any, len(m))
		for k, v := range m {
			out[k] = RedactValue(k, v)
		}
		return out
	}
	return map[string]any{"redacted": true}
}

// RedactProjectionContext scrubs a projector-style context map (items list).
func RedactProjectionContext(context map[string]any) map[string]any {
	if context == nil {
		return map[string]any{"redacted": true}
	}
	rawItems, ok := context["items"].([]any)
	if !ok {
		// Free-form dict: redact top-level keys.
		out := make(map[string]any, len(context))
		for k, v := range context {
			out[k] = RedactValue(k, v)
		}
		out["redacted"] = true
		return out
	}
	newItems := make([]any, 0, len(rawItems))
	for _, it := range rawItems {
		item, ok := it.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := item["kind"].(string)
		cp := make(map[string]any, len(item))
		for k, v := range item {
			cp[k] = v
		}
		cp["payload"] = RedactPayload(kind, item["payload"])
		newItems = append(newItems, cp)
	}
	out := make(map[string]any, len(context)+1)
	for k, v := range context {
		out[k] = v
	}
	out["items"] = newItems
	out["redacted"] = true
	return out
}
