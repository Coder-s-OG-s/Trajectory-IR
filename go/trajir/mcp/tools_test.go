package mcp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	trajirmcp "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/mcp"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mutateZipMember rewrites one member of the zip at path, keeping others intact.
func mutateZipMember(t *testing.T, path, name string, mutate func([]byte) []byte) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == name {
			data = mutate(data)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func seedTrajectory(t *testing.T, workDir string) {
	t.Helper()
	tr, err := client.OpenTrajectory("demo", "mcp-traj", client.Options{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	step := 1
	if _, err := tr.SealDecision(step, map[string]any{
		"plan": map[string]any{"tool_calls": []any{}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tr.CommitStep(step, 1); err != nil {
		t.Fatal(err)
	}
}

func TestToolsViaInMemoryMCP(t *testing.T) {
	work := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", work)
	seedTrajectory(t, work)

	ctx := context.Background()
	server := trajirmcp.NewServer()
	t1, t2 := mcp.NewInMemoryTransports()

	serverSession, err := server.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	// List tools
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		"trajectory_status",
		"trajectory_export_tir",
		"trajectory_import_tir",
		"trajectory_verify_signature",
	} {
		if !names[want] {
			t.Fatalf("missing tool %q in %#v", want, names)
		}
	}

	// status
	st, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_status",
		Arguments: map[string]any{
			"work_dir":      work,
			"tenant_id":     "demo",
			"trajectory_id": "mcp-traj",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.IsError {
		t.Fatalf("status error: %+v", st)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(mustJSON(t, st.StructuredContent)), &status); err != nil {
		// StructuredContent may already be map
		if m, ok := st.StructuredContent.(map[string]any); ok {
			status = m
		} else {
			t.Fatalf("status content: %T %+v", st.StructuredContent, st.StructuredContent)
		}
	}
	if intFromAny(status["node_count"]) < 1 {
		t.Fatalf("node_count=%v", status["node_count"])
	}

	// export (relative dest under workspace)
	ex, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_export_tir",
		Arguments: map[string]any{
			"work_dir":      work,
			"tenant_id":     "demo",
			"trajectory_id": "mcp-traj",
			"dest":          "pkg.tir",
			"mode":          "thin",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.IsError {
		t.Fatalf("export error: %+v", ex)
	}
	dest := filepath.Join(work, "pkg.tir")

	// path escape must fail
	escape, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_import_tir",
		Arguments: map[string]any{"path": filepath.Join(t.TempDir(), "evil.tir")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !escape.IsError {
		t.Fatal("expected path escape to fail closed")
	}

	// import
	im, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_import_tir",
		Arguments: map[string]any{"path": dest},
	})
	if err != nil {
		t.Fatal(err)
	}
	if im.IsError {
		t.Fatalf("import error: %+v", im)
	}

	// verify unsigned
	vr, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_verify_signature",
		Arguments: map[string]any{"path": dest},
	})
	if err != nil {
		t.Fatal(err)
	}
	if vr.IsError {
		t.Fatalf("verify error: %+v", vr)
	}

	// bad mode / missing ids
	if bad, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_export_tir",
		Arguments: map[string]any{
			"work_dir": work, "tenant_id": "demo", "trajectory_id": "mcp-traj",
			"dest": "x.tir", "mode": "bogus",
		},
	}); err != nil {
		t.Fatal(err)
	} else if !bad.IsError {
		t.Fatal("expected bad mode error")
	}

	// require_signature on unsigned reports a structured failure (not a
	// protocol error), so status:"failed" actually reaches the wire.
	if strict, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_verify_signature",
		Arguments: map[string]any{"path": dest, "require_signature": true},
	}); err != nil {
		t.Fatal(err)
	} else if strict.IsError {
		t.Fatalf("expected structured failure, got protocol error: %+v", strict)
	} else if m, ok := strict.StructuredContent.(map[string]any); !ok || m["status"] != "failed" {
		t.Fatalf("expected status=failed in structured content, got %+v", strict.StructuredContent)
	}

	// missing tenant
	if miss, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_status",
		Arguments: map[string]any{"work_dir": work, "tenant_id": "", "trajectory_id": "mcp-traj"},
	}); err != nil {
		t.Fatal(err)
	} else if !miss.IsError {
		t.Fatal("expected missing tenant error")
	}

	// empty dest
	if emptyDest, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_export_tir",
		Arguments: map[string]any{
			"work_dir": work, "tenant_id": "demo", "trajectory_id": "mcp-traj", "dest": "",
		},
	}); err != nil {
		t.Fatal(err)
	} else if !emptyDest.IsError {
		t.Fatal("expected empty dest error")
	}

	// fat mode with zero artifacts (no ArtifactRefs supplied via the tool) succeeds.
	if fat, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_export_tir",
		Arguments: map[string]any{
			"work_dir": work, "tenant_id": "demo", "trajectory_id": "mcp-traj",
			"dest": "fat.tir", "mode": "fat",
		},
	}); err != nil {
		t.Fatal(err)
	} else if fat.IsError {
		t.Fatalf("fat export failed: %+v", fat)
	}

	// signed package verify success path
	seed := sha256.Sum256([]byte("trajir-mcp-test-seed"))
	priv := ed25519.NewKeyFromSeed(seed[:])
	if err := tir.Sign(dest, priv, tir.SignerMeta{ID: "mcp-test"}); err != nil {
		t.Fatal(err)
	}
	if signed, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "trajectory_verify_signature",
		Arguments: map[string]any{"path": "pkg.tir"},
	}); err != nil {
		t.Fatal(err)
	} else if signed.IsError {
		t.Fatalf("signed verify failed: %+v", signed)
	}

	// tampered signed package reports status:"failed" (structured, not a
	// protocol error), distinguishing it from "unsigned".
	mutateZipMember(t, dest, "COMPAT.json", func(b []byte) []byte {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		m["min_runtime"] = "9.9.9"
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return append(out, '\n')
	})
	if tampered, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "trajectory_verify_signature",
		Arguments: map[string]any{"path": "pkg.tir"},
	}); err != nil {
		t.Fatal(err)
	} else if tampered.IsError {
		t.Fatalf("expected structured failure, got protocol error: %+v", tampered)
	} else if m, ok := tampered.StructuredContent.(map[string]any); !ok || m["status"] != "failed" {
		t.Fatalf("expected status=failed in structured content, got %+v", tampered.StructuredContent)
	}
}

func TestRunStdioNilContext(t *testing.T) {
	// Do not block on real stdio; just ensure NewServer is usable for RunStdio wiring.
	if trajirmcp.NewServer() == nil {
		t.Fatal("nil server")
	}
}

func TestStatusRejectsSymlinkNodesDB(t *testing.T) {
	work := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", work)
	proj := filepath.Join(work, "p")
	if err := os.Mkdir(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "nodes.sqlite")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(proj, "nodes.sqlite")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	ctx := context.Background()
	server := trajirmcp.NewServer()
	t1, t2 := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cl := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "v0"}, nil)
	cs, err := cl.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_status",
		Arguments: map[string]any{
			"work_dir": proj, "tenant_id": "demo", "trajectory_id": "t",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected symlink nodes.sqlite to fail status")
	}
}


func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
