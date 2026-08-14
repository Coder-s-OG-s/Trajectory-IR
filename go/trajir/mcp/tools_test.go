package mcp_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	trajirmcp "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

	// export
	dest := filepath.Join(work, "pkg.tir")
	ex, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "trajectory_export_tir",
		Arguments: map[string]any{
			"work_dir":      work,
			"tenant_id":     "demo",
			"trajectory_id": "mcp-traj",
			"dest":          dest,
			"mode":          "thin",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ex.IsError {
		t.Fatalf("export error: %+v", ex)
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
