package nodes_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
)

type vectorFile struct {
	Version int `json:"version"`
	Cases   []struct {
		Name         string         `json:"name"`
		TenantID     string         `json:"tenant_id"`
		TrajectoryID string         `json:"trajectory_id"`
		StepN        *int           `json:"step_n"`
		Seq          int            `json:"seq"`
		Kind         string         `json:"kind"`
		Payload      map[string]any `json:"payload"`
		PayloadHash  string         `json:"payload_hash"`
		NodeID       string         `json:"node_id"`
	} `json:"cases"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// go/trajir/nodes -> repo root testdata/
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	path := filepath.Join(root, "testdata", "hash_vectors.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var vf vectorFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	if len(vf.Cases) == 0 {
		t.Fatal("no cases in hash_vectors.json")
	}
	return vf
}

func TestHashVectorsMatchPythonGoldens(t *testing.T) {
	vf := loadVectors(t)
	for _, c := range vf.Cases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			ph, err := nodes.PayloadHash(c.Payload)
			if err != nil {
				t.Fatalf("PayloadHash: %v", err)
			}
			if ph != c.PayloadHash {
				t.Fatalf("payload_hash\n got %s\nwant %s", ph, c.PayloadHash)
			}
			id, err := nodes.NodeID(c.TenantID, c.TrajectoryID, c.StepN, c.Seq, c.Kind, ph)
			if err != nil {
				t.Fatalf("NodeID: %v", err)
			}
			if id != c.NodeID {
				t.Fatalf("node_id\n got %s\nwant %s", id, c.NodeID)
			}
			n, err := nodes.NewNode(c.Kind, c.TrajectoryID, c.TenantID, c.StepN, c.Seq, c.Payload)
			if err != nil {
				t.Fatalf("NewNode: %v", err)
			}
			if n.PHash != c.PayloadHash || n.ID != c.NodeID {
				t.Fatalf("NewNode mismatch phash/id")
			}
		})
	}
}

func TestKeyOrderIndependent(t *testing.T) {
	a := map[string]any{"a": float64(1), "b": float64(2)}
	b := map[string]any{"b": float64(2), "a": float64(1)}
	ha, err := nodes.PayloadHash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := nodes.PayloadHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatalf("key order changed hash: %s vs %s", ha, hb)
	}
}

func TestTSFieldRejected(t *testing.T) {
	_, err := nodes.PayloadHash(map[string]any{"ts": float64(1)})
	if err == nil {
		t.Fatal("expected error for ts in payload")
	}
}

func TestUnknownKindRejected(t *testing.T) {
	step := 1
	_, err := nodes.NewNode("NOT_A_KIND", "t1", "demo", &step, 1, map[string]any{})
	if err == nil {
		t.Fatal("expected unknown kind error")
	}
}

func TestValidateIDComponentRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"invalid utf8", string([]byte{0xff, 0xfe})},
		{"pipe delimiter", "a|b"},
		{"control char", "a\x00b"},
		{"too long", strings.Repeat("x", 513)},
		{"too long multibyte", strings.Repeat("中", 513)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if err := nodes.ValidateIDComponent("field", c.value); err == nil {
				t.Fatalf("expected error for %q", c.value)
			}
		})
	}
}

func TestValidateIDComponentAccepts(t *testing.T) {
	if err := nodes.ValidateIDComponent("field", "ok-value"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIDComponentCountsCharactersNotBytes(t *testing.T) {
	// Python len() counts code points, so 512 multibyte characters must pass here too.
	for _, value := range []string{strings.Repeat("中", 512), strings.Repeat("😀", 512)} {
		if err := nodes.ValidateIDComponent("tenant_id", value); err != nil {
			t.Fatalf("len=%d bytes: %v", len(value), err)
		}
	}
}

func ptrInt64(n int64) *int64 {
	return &n
}

type hiddenBigInt struct {
	N      int64 `json:"n"`
	Hidden int64 `json:"-"`
}

func safeWithHiddenBigInt() hiddenBigInt {
	return hiddenBigInt{N: 1, Hidden: 1 << 60}
}

func TestPayloadHashRejectsUnmarshalableValue(t *testing.T) {
	_, err := nodes.PayloadHash(map[string]any{"bad": math.NaN()})
	if err == nil {
		t.Fatal("expected json marshal error for NaN")
	}
}

func TestDecodeObjectPreservesIntegerLiteral(t *testing.T) {
	obj, err := nodes.DecodeObject([]byte(`{"n":9007199254740993}`))
	if err != nil {
		t.Fatal(err)
	}
	num, ok := obj["n"].(json.Number)
	if !ok || num != "9007199254740993" {
		t.Fatalf("n=%#v", obj["n"])
	}
	if _, err := nodes.DecodeObject([]byte(`{"n":1}{"x":2}`)); err == nil {
		t.Fatal("expected trailing value error")
	}
	if _, err := nodes.DecodeObject([]byte(`{`)); err == nil {
		t.Fatal("expected syntax error")
	}
	nullObj, err := nodes.DecodeObject([]byte("null"))
	if err != nil {
		t.Fatal(err)
	}
	if nullObj != nil {
		t.Fatalf("null payload=%#v", nullObj)
	}
}

func TestPayloadHashSafeIntegerMatchesPython(t *testing.T) {
	hi, err := nodes.PayloadHash(map[string]any{"n": int64(1<<53 - 1)})
	if err != nil {
		t.Fatal(err)
	}
	if hi != "e1da48c6a6089f06ecb4e0a2259e658e3786b2420f52baccdf929ec6460d7b41" {
		t.Fatalf("2^53-1 hash %s", hi)
	}
	lo, err := nodes.PayloadHash(map[string]any{"n": int64(-(1<<53 - 1))})
	if err != nil {
		t.Fatal(err)
	}
	if lo != "d49d713821fc149f81ef6ca8054beeba696f5da052f0ab3e2d773808c5a9d625" {
		t.Fatalf("-(2^53-1) hash %s", lo)
	}
	if _, err := nodes.PayloadHash(map[string]any{"n": uint64(1)}); err != nil {
		t.Fatal(err)
	}
	asNumber, err := nodes.PayloadHash(map[string]any{"n": json.Number("9007199254740991")})
	if err != nil {
		t.Fatal(err)
	}
	if asNumber != hi {
		t.Fatalf("json.Number hash %s want %s", asNumber, hi)
	}
}

func TestPayloadHashLargeFloatFollowsJCS(t *testing.T) {
	dotted, err := nodes.PayloadHash(map[string]any{"n": json.Number("9007199254740992.0")})
	if err != nil {
		t.Fatal(err)
	}
	if dotted != "66c87d9cb3014e05a11baa97df62282d89d425f22ee15816577c84534e2ef1bb" {
		t.Fatalf("float literal hash %s", dotted)
	}
	if _, err := nodes.PayloadHash(map[string]any{"n": 1.5}); err != nil {
		t.Fatal(err)
	}
	got, err := nodes.PayloadHash(map[string]any{"n": float64(1e18)})
	if err != nil {
		t.Fatal(err)
	}
	if got != "d9fcf531f15705cb9f48516582481ee4c597252951ca1bd994986e731136b784" {
		t.Fatalf("float64(1e18) hash %s", got)
	}
	if _, err := nodes.PayloadHash(map[string]any{"n": float64(int64(1 << 53))}); err != nil {
		t.Fatal(err)
	}
}

func TestPayloadHashRejectsIntegersOutsideSafeRange(t *testing.T) {
	cases := []struct {
		name string
		n    any
	}{
		{"int64 at 2^53", int64(1 << 53)},
		{"int64 just above", int64((1 << 53) + 1)},
		{"int64 at 2^60", int64(1 << 60)},
		{"int64 just below negative bound", int64(-(1 << 53))},
		{"uint64", uint64(1 << 53)},
		{"json number", json.Number("9007199254740993")},
		{"wide json number", json.Number("1152921504606846977")},
		{"nested", map[string]any{"k": []any{int64((1 << 53) + 1)}}},
		{"int64 slice", []int64{1 << 53}},
		{"pointer", ptrInt64(1 << 53)},
		{"raw message", json.RawMessage(`{"n":9007199254740993}`)},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			_, err := nodes.PayloadHash(map[string]any{"n": c.n})
			if err == nil || !strings.Contains(err.Error(), "safe integer") {
				t.Fatalf("err=%v", err)
			}
		})
	}

	_, errHi := nodes.PayloadHash(map[string]any{"n": int64(1 << 53)})
	_, errLo := nodes.PayloadHash(map[string]any{"n": int64((1 << 53) + 1)})
	if errHi == nil || errLo == nil {
		t.Fatal("distinct integers above 2^53-1 hashed")
	}

	if _, err := nodes.PayloadHash(map[string]any{"row": safeWithHiddenBigInt()}); err != nil {
		t.Fatal(err)
	}
}

func TestNodeIDRejectsInvalidComponents(t *testing.T) {
	if _, err := nodes.NodeID("", "t1", nil, 0, "INPUT", "h"); err == nil {
		t.Fatal("expected error for empty tenant_id")
	}
	if _, err := nodes.NodeID("demo", "", nil, 0, "INPUT", "h"); err == nil {
		t.Fatal("expected error for empty trajectory_id")
	}
	if _, err := nodes.NodeID("demo", "t1", nil, 0, "TOOL|CALL", "h"); err == nil {
		t.Fatal("expected error for pipe in kind")
	}
	if _, err := nodes.NodeID("demo", "t1", nil, 0, "INPUT", "a|b"); err == nil {
		t.Fatal("expected error for pipe in payload_hash")
	}
	if _, err := nodes.NodeID("demo", "t1", nil, 0, "INPUT", ""); err == nil {
		t.Fatal("expected error for empty payload_hash")
	}
}

func TestNewNodeRejectsInvalidComponents(t *testing.T) {
	if _, err := nodes.NewNode("INPUT", "t1", "", nil, 0, nil); err == nil {
		t.Fatal("expected error for empty tenant_id")
	}
	if _, err := nodes.NewNode("INPUT", "", "demo", nil, 0, nil); err == nil {
		t.Fatal("expected error for empty trajectory_id")
	}
}

func TestNewNodePropagatesPayloadHashError(t *testing.T) {
	_, err := nodes.NewNode("INPUT", "t1", "demo", nil, 0, map[string]any{"ts": 1.0})
	if err == nil {
		t.Fatal("expected error for ts key in payload")
	}
}

func TestNewNodeDefaultsNilPayload(t *testing.T) {
	step := 1
	n, err := nodes.NewNode("INPUT", "t1", "demo", &step, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.Payload == nil {
		t.Fatal("expected non-nil default payload")
	}
}

func TestWallClockTSDoesNotAffectID(t *testing.T) {
	step := 1
	payload := map[string]any{"a": float64(1)}
	n1, err := nodes.NewNode("STATE_SET", "t1", "demo", &step, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	n2, err := nodes.NewNode("STATE_SET", "t1", "demo", &step, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	if n1.ID != n2.ID {
		t.Fatalf("id changed with wall clock: %s vs %s", n1.ID, n2.ID)
	}
}

func TestDecodePayloadJSONKeepsIntegersExact(t *testing.T) {
	p, err := nodes.DecodePayloadJSON(`{"big":9007199254740993,"f":1.5,"nested":{"n":42}}`)
	if err != nil {
		t.Fatal(err)
	}
	big, ok := p["big"].(json.Number)
	if !ok || big.String() != "9007199254740993" {
		t.Fatalf("big=%#v, want json.Number 9007199254740993", p["big"])
	}
	if n, ok := p["nested"].(map[string]any)["n"].(json.Number); !ok || n.String() != "42" {
		t.Fatalf("nested n=%#v", p["nested"])
	}
	// Re-encoding must not turn the integer into 9007199254740992 or 9.007e15.
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"big":9007199254740993`) {
		t.Fatalf("re-encoded payload lost precision: %s", out)
	}
}

func TestDecodePayloadJSONRejectsMalformedAndTrailingData(t *testing.T) {
	for _, raw := range []string{``, `{"a":1} garbage`, `{"a":1}{"b":2}`, `{"a":1} 2`, `[1]`, `{"a":`} {
		if _, err := nodes.DecodePayloadJSON(raw); err == nil {
			t.Errorf("DecodePayloadJSON(%q) = nil error, want rejection", raw)
		}
	}
	if p, err := nodes.DecodePayloadJSON(" {\"a\":1}\n"); err != nil || len(p) != 1 {
		t.Fatalf("surrounding whitespace must be accepted: p=%v err=%v", p, err)
	}
}
