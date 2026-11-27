package mcp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		Name:      "trajectory_import_tir",
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
		Name:      "trajectory_import_tir",
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
		Name:      "trajectory_verify_signature",
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
		Name:      "trajectory_verify_signature",
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
		Name:      "trajectory_status",
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

func exportViaMCP(t *testing.T, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()
	server := trajirmcp.NewServer()
	t1, t2 := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "trajectory_export_tir",
		Arguments: args,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func toolErrorText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	if b.Len() == 0 {
		return fmt.Sprint(res)
	}
	return b.String()
}

// A missing parent under work_dir is created. The old failure was a temp
// file open in a folder that was never made.
func TestExportCreatesDestParentUnderWorkDir(t *testing.T) {
	work := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", work)
	seedTrajectory(t, work)

	res := exportViaMCP(t, map[string]any{
		"work_dir":      work,
		"tenant_id":     "demo",
		"trajectory_id": "mcp-traj",
		"dest":          "evidence/out/pkg.tir",
	})
	if res.IsError {
		t.Fatalf("export error: %s", toolErrorText(res))
	}
	if _, err := os.Stat(filepath.Join(work, "evidence", "out", "pkg.tir")); err != nil {
		t.Fatalf("exported file missing: %v", err)
	}
}

// work_dir is a real subdirectory. A dest that repeats that path is
// rejected, and the error names the path to pass instead. The exact
// work_dir path must not be offered back as the hint.
func TestExportRejectsDestStartingWithWorkDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", root)
	work := filepath.Join(root, "sub", "agent")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	seedTrajectory(t, work)

	cases := []struct {
		dest string
		hint string
	}{
		{dest: "sub/agent/out/pkg.tir", hint: `"out/pkg.tir"`},
		{dest: "sub/agent", hint: `"out.tir"`},
		{dest: "sub/agent/", hint: `"out.tir"`},
	}
	for _, tc := range cases {
		res := exportViaMCP(t, map[string]any{
			"work_dir":      "sub/agent",
			"tenant_id":     "demo",
			"trajectory_id": "mcp-traj",
			"dest":          tc.dest,
		})
		if !res.IsError {
			t.Fatalf("dest %q: want an error", tc.dest)
		}
		msg := toolErrorText(res)
		if !strings.Contains(msg, "relative to work_dir") || !strings.Contains(msg, tc.hint) {
			t.Fatalf("dest %q: error %q should tell the caller to pass %s", tc.dest, msg, tc.hint)
		}
	}
}

// A normal relative dest lands under the subdirectory work_dir. A sibling
// name such as sub/agent-extra is not treated as the work_dir prefix.
func TestExportRelativeDestUnderSubdirWorkDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", root)
	work := filepath.Join(root, "sub", "agent")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	seedTrajectory(t, work)

	res := exportViaMCP(t, map[string]any{
		"work_dir":      "sub/agent",
		"tenant_id":     "demo",
		"trajectory_id": "mcp-traj",
		"dest":          "out/pkg.tir",
	})
	if res.IsError {
		t.Fatalf("export error: %s", toolErrorText(res))
	}
	if _, err := os.Stat(filepath.Join(work, "out", "pkg.tir")); err != nil {
		t.Fatalf("exported file missing under work_dir: %v", err)
	}

	sibling := exportViaMCP(t, map[string]any{
		"work_dir":      "sub/agent",
		"tenant_id":     "demo",
		"trajectory_id": "mcp-traj",
		"dest":          "sub/agent-extra/pkg.tir",
	})
	if sibling.IsError {
		t.Fatalf("sibling prefix should export: %s", toolErrorText(sibling))
	}
	got := filepath.Join(work, "sub", "agent-extra", "pkg.tir")
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("sibling dest missing: %v", err)
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

func fileURI(abs string) string {
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return "file://" + slash
}

func connectWithRoots(t *testing.T, roots ...string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := trajirmcp.NewServer()
	left, right := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, left, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	for _, root := range roots {
		client.AddRoots(&mcp.Root{URI: fileURI(root)})
	}
	clientSession, err := client.Connect(ctx, right, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })
	return clientSession
}

func statusCall(t *testing.T, session *mcp.ClientSession, work string) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
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
	return res
}

func TestStatusUsesSingleClientRoot(t *testing.T) {
	work := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	seedTrajectory(t, work)

	res := statusCall(t, connectWithRoots(t, work), work)
	if res.IsError {
		t.Fatalf("status error: %+v", res)
	}
	if intFromAny(asMap(t, res)["node_count"]) < 1 {
		t.Fatalf("node_count=%v", res.StructuredContent)
	}
}

func TestStatusFailsClosedWithoutARoot(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	t.Setenv("TRAJIR_MCP_ROOT", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	seedTrajectory(t, work)

	res := statusCall(t, connectWithRoots(t), work)
	if !res.IsError {
		t.Fatal("expected failure when no workspace root is configured")
	}
	if !strings.Contains(toolText(res), "TRAJIR_MCP_ROOT") {
		t.Fatalf("error = %s", toolText(res))
	}
}

func TestConfiguredRootWinsOverClientRoots(t *testing.T) {
	work := t.TempDir()
	other := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", work)
	t.Setenv("CLAUDE_PROJECT_DIR", other)
	seedTrajectory(t, work)
	seedTrajectory(t, other)

	session := connectWithRoots(t, other)
	outside := statusCall(t, session, other)
	if !outside.IsError {
		t.Fatal("client root must not widen past TRAJIR_MCP_ROOT")
	}
	inside := statusCall(t, session, work)
	if inside.IsError {
		t.Fatalf("status inside configured root: %+v", inside)
	}
}

func TestClaudeProjectDirIsTheRootWhenUnset(t *testing.T) {
	work := t.TempDir()
	other := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", "")
	t.Setenv("CLAUDE_PROJECT_DIR", work)
	seedTrajectory(t, work)

	session := connectWithRoots(t, other)
	outside := statusCall(t, session, other)
	if !outside.IsError {
		t.Fatal("a client root must not replace CLAUDE_PROJECT_DIR")
	}
	inside := statusCall(t, session, work)
	if inside.IsError {
		t.Fatalf("status under CLAUDE_PROJECT_DIR: %+v", inside)
	}
}

func TestTwoClientRootsFailClosed(t *testing.T) {
	work := t.TempDir()
	other := t.TempDir()
	t.Setenv("TRAJIR_MCP_ROOT", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	seedTrajectory(t, work)

	res := statusCall(t, connectWithRoots(t, work, other), work)
	if !res.IsError || !strings.Contains(toolText(res), "exactly one workspace directory") {
		t.Fatalf("error = %s", toolText(res))
	}
}

func asMap(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if m, ok := res.StructuredContent.(map[string]any); ok {
		return m
	}
	var m map[string]any
	raw, ok := res.StructuredContent.(json.RawMessage)
	if !ok {
		t.Fatalf("status content: %T", res.StructuredContent)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func toolText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}
