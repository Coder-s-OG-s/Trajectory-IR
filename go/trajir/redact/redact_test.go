package redact_test

import (
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/redact"
)

func TestRedactProjectionContext(t *testing.T) {
	ctx := map[string]any{
		"items": []any{
			map[string]any{
				"id":   "n1",
				"kind": "TOOL_CALL",
				"payload": map[string]any{
					"args": map[string]any{
						"note":    "found it: AKIAABCDEFGHIJKLMNOP",
						"message": "benign",
					},
				},
			},
			map[string]any{
				"id":   "c1",
				"kind": "CONSTRAINT",
				"payload": map[string]any{
					"rule":    "keep",
					"api_key": "x",
				},
			},
			map[string]any{
				"id":      "th1",
				"kind":    "THOUGHT",
				"payload": map[string]any{"text": "private"},
			},
		},
	}
	out := redact.RedactProjectionContext(ctx)
	if out["redacted"] != true {
		t.Fatal("expected redacted flag")
	}
	items := out["items"].([]any)
	byID := map[string]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		byID[m["id"].(string)] = m
	}
	args := byID["n1"]["payload"].(map[string]any)["args"].(map[string]any)
	if args["note"] != redact.Redacted {
		t.Fatalf("note=%v", args["note"])
	}
	if args["message"] != "benign" {
		t.Fatalf("message=%v", args["message"])
	}
	cp := byID["c1"]["payload"].(map[string]any)
	if cp["rule"] != "keep" || cp["api_key"] != redact.Redacted {
		t.Fatalf("constraint payload=%v", cp)
	}
	tp := byID["th1"]["payload"].(map[string]any)
	if tp["redacted"] != true {
		t.Fatalf("thought=%v", tp)
	}
}
