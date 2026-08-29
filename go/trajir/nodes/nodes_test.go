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

func TestPayloadHashRejectsUnmarshalableValue(t *testing.T) {
	_, err := nodes.PayloadHash(map[string]any{"bad": math.NaN()})
	if err == nil {
		t.Fatal("expected json marshal error for NaN")
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
