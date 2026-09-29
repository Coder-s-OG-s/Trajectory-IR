package resume_test

import (
	"path/filepath"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

func TestIdempotencyKeyFormat(t *testing.T) {
	got := resume.IdempotencyKey("traj-1", 1, 2)
	if got != "traj-1:1:2" {
		t.Fatalf("got %q", got)
	}
}

func TestIdempotencyKeyStableForSealedSlot(t *testing.T) {
	a := resume.IdempotencyKey("t", 3, 4)
	b := resume.IdempotencyKey("t", 3, 4)
	if a != b {
		t.Fatalf("unstable key %q vs %q", a, b)
	}
}

func TestIdempotencyKeyChangesWithSlot(t *testing.T) {
	if resume.IdempotencyKey("t", 1, 2) == resume.IdempotencyKey("t", 1, 4) {
		t.Fatal("seq must be part of the key")
	}
	if resume.IdempotencyKey("t", 1, 2) == resume.IdempotencyKey("t", 2, 2) {
		t.Fatal("step must be part of the key")
	}
	if resume.IdempotencyKey("a", 1, 2) == resume.IdempotencyKey("b", 1, 2) {
		t.Fatal("trajectory id must be part of the key")
	}
}

func TestGatedToolCallRecordsKeyWithoutInjectingIntoArgs(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	var seen map[string]any
	tool := func(args map[string]any) (any, error) {
		seen = args
		return map[string]any{"ok": true}, nil
	}
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy_server", tool)
	if _, err := gated(map[string]any{"version": "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if seen["version"] != "1.0.0" {
		t.Fatalf("args=%#v", seen)
	}
	if _, ok := seen["idempotency_key"]; ok {
		t.Fatalf("must not inject key into tool args: %#v", seen)
	}

	payload := toolCallPayload(t, nl, "t1", "demo", 2)
	if payload["tool"] != "deploy_server" {
		t.Fatalf("payload=%#v", payload)
	}
	if payload["idempotency_key"] != "t1:1:2" {
		t.Fatalf("idempotency_key=%#v", payload["idempotency_key"])
	}
	args, _ := payload["args"].(map[string]any)
	if args["version"] != "1.0.0" {
		t.Fatalf("logged args=%#v", args)
	}
}

func TestPlainToolCallRecordsKey(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	plain := resume.MakePlainToolCall(nl, "t1", "demo", 1, 2, "echo", func(args map[string]any) (any, error) {
		return args["msg"], nil
	})
	if _, err := plain(map[string]any{"msg": "hi"}); err != nil {
		t.Fatal(err)
	}
	payload := toolCallPayload(t, nl, "t1", "demo", 2)
	if payload["idempotency_key"] != resume.IdempotencyKey("t1", 1, 2) {
		t.Fatalf("payload=%#v", payload)
	}
}

func toolCallPayload(t *testing.T, nl *nodelog.NodeLog, trajectoryID, tenantID string, seq int) map[string]any {
	t.Helper()
	rows, err := nl.ListNodes(trajectoryID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row["kind"] == "TOOL_CALL" && row["seq"] == seq {
			p, _ := row["payload"].(map[string]any)
			if p == nil {
				t.Fatal("nil payload")
			}
			return p
		}
	}
	t.Fatalf("no TOOL_CALL at seq=%d", seq)
	return nil
}
