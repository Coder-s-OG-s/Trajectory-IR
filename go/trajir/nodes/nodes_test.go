package nodes_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
