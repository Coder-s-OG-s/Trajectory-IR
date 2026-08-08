package redact_test

import (
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/redact"
)

func TestRedactValueNonStringLeafPassesThrough(t *testing.T) {
	if redact.RedactValue("count", 42) != 42 {
		t.Fatal("non-string leaf should pass through unchanged")
	}
	if redact.RedactValue("ok", true) != true {
		t.Fatal("bool leaf should pass through unchanged")
	}
}

func TestRedactValueListRedactsSecretShapedStrings(t *testing.T) {
	out := redact.RedactValue("notes", []any{"benign", "AKIAABCDEFGHIJKLMNOP"})
	list := out.([]any)
	if list[0] != "benign" {
		t.Fatalf("list[0]=%v", list[0])
	}
	if list[1] != redact.Redacted {
		t.Fatalf("list[1]=%v", list[1])
	}
}

func TestRedactPayloadNonMapPayload(t *testing.T) {
	out := redact.RedactPayload("TOOL_CALL", "not a map")
	if out["redacted"] != true {
		t.Fatalf("out=%v", out)
	}
}

func TestRedactProjectionContextNil(t *testing.T) {
	out := redact.RedactProjectionContext(nil)
	if out["redacted"] != true {
		t.Fatalf("out=%v", out)
	}
}

func TestRedactProjectionContextFreeFormDict(t *testing.T) {
	out := redact.RedactProjectionContext(map[string]any{
		"api_key": "shh",
		"note":    "benign",
	})
	if out["redacted"] != true {
		t.Fatalf("out=%v", out)
	}
	if out["api_key"] != redact.Redacted {
		t.Fatalf("api_key=%v", out["api_key"])
	}
	if out["note"] != "benign" {
		t.Fatalf("note=%v", out["note"])
	}
}

func TestRedactProjectionContextSkipsNonMapItems(t *testing.T) {
	out := redact.RedactProjectionContext(map[string]any{
		"items": []any{"not-a-map", map[string]any{"id": "n1", "kind": "TOOL_CALL", "payload": map[string]any{}}},
	})
	items := out["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%v want len 1 (non-map skipped)", items)
	}
}

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
