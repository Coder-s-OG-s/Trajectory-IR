package resume_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
)

type idempotencyVectorFile struct {
	Cases []struct {
		Name           string `json:"name"`
		TenantID       string `json:"tenant_id"`
		TrajectoryID   string `json:"trajectory_id"`
		StepN          int    `json:"step_n"`
		Seq            int    `json:"seq"`
		IdempotencyKey string `json:"idempotency_key"`
	} `json:"cases"`
}

func TestIdempotencyKeyMatchesCommittedVectors(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "testdata", "idempotency_vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vf idempotencyVectorFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		t.Fatal(err)
	}
	if len(vf.Cases) == 0 {
		t.Fatal("no cases")
	}
	for _, c := range vf.Cases {
		got := resume.IdempotencyKey(c.TenantID, c.TrajectoryID, c.StepN, c.Seq)
		if got != c.IdempotencyKey {
			t.Fatalf("%s: got %q want %q", c.Name, got, c.IdempotencyKey)
		}
		if len(got) != 64 {
			t.Fatalf("%s: key length %d", c.Name, len(got))
		}
	}
}

func TestIdempotencyKeyStableForSealedSlot(t *testing.T) {
	a := resume.IdempotencyKey("demo", "t", 3, 4)
	b := resume.IdempotencyKey("demo", "t", 3, 4)
	if a != b {
		t.Fatalf("unstable key %q vs %q", a, b)
	}
}

func TestIdempotencyKeyChangesWithSlotOrTenant(t *testing.T) {
	base := resume.IdempotencyKey("demo", "t", 1, 2)
	if base == resume.IdempotencyKey("demo", "t", 1, 4) {
		t.Fatal("seq must be part of the key")
	}
	if base == resume.IdempotencyKey("demo", "t", 2, 2) {
		t.Fatal("step must be part of the key")
	}
	if base == resume.IdempotencyKey("demo", "b", 1, 2) {
		t.Fatal("trajectory id must be part of the key")
	}
	if base == resume.IdempotencyKey("other", "t", 1, 2) {
		t.Fatal("tenant must be part of the key")
	}
}

func TestIdempotencyKeyNulInsideIdDoesNotAlias(t *testing.T) {
	a := resume.IdempotencyKey("a", "b\x00c", 1, 2)
	b := resume.IdempotencyKey("a\x00b", "c", 1, 2)
	if a == b {
		t.Fatal("NUL inside a tenant or trajectory id must not alias another pair")
	}
}

func TestIdempotencyKeyHeader(t *testing.T) {
	key := resume.IdempotencyKey("demo", "t1", 1, 2)
	h := resume.IdempotencyKeyHeader(key)
	if h["Idempotency-Key"] != key {
		t.Fatalf("header=%v", h)
	}
}

func TestGatedToolCallRecordsKeyWithoutInjectingIntoArgs(t *testing.T) {
	nl, err := nodelog.Open(filepath.Join(t.TempDir(), "nodes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nl.Close() })

	var seen map[string]any
	var seenMeta resume.CallMeta
	tool := resume.Tool{
		Name: "deploy_server",
		FnWithMeta: func(args map[string]any, meta resume.CallMeta) (any, error) {
			seen = args
			seenMeta = meta
			return map[string]any{"ok": true}, nil
		},
	}
	meta := resume.NewCallMeta("demo", "t1", 1, 2)
	gated := resume.MakeGatedToolCall(nl, "t1", "demo", 1, 2, "deploy_server", resume.BoundToolFn(tool, meta))
	if _, err := gated(map[string]any{"version": "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if seen["version"] != "1.0.0" {
		t.Fatalf("args=%#v", seen)
	}
	if _, ok := seen["idempotency_key"]; ok {
		t.Fatalf("must not inject key into tool args: %#v", seen)
	}
	want := resume.IdempotencyKey("demo", "t1", 1, 2)
	if seenMeta.IdempotencyKey != want {
		t.Fatalf("meta key=%q want %q", seenMeta.IdempotencyKey, want)
	}

	payload := toolCallPayload(t, nl, "t1", "demo", 2)
	if payload["tool"] != "deploy_server" {
		t.Fatalf("payload=%#v", payload)
	}
	if payload["idempotency_key"] != want {
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
	if payload["idempotency_key"] != resume.IdempotencyKey("demo", "t1", 1, 2) {
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
