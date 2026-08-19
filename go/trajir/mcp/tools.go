package mcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Common path args for tools that touch a local workdir NodeLog.
type workdirArgs struct {
	WorkDir      string `json:"work_dir" jsonschema:"directory under TRAJIR_MCP_ROOT (or cwd) holding nodes.sqlite"`
	TenantID     string `json:"tenant_id" jsonschema:"tenant id for the trajectory"`
	TrajectoryID string `json:"trajectory_id" jsonschema:"trajectory id"`
}

type statusOut struct {
	WorkDir      string         `json:"work_dir"`
	NodesPath    string         `json:"nodes_path"`
	TenantID     string         `json:"tenant_id"`
	TrajectoryID string         `json:"trajectory_id"`
	NodeCount    int            `json:"node_count"`
	SealCount    int            `json:"seal_count"`
	CountsByKind map[string]int `json:"counts_by_kind"`
	Kinds        []string       `json:"kinds"`
}

func toolStatus(ctx context.Context, _ *mcp.CallToolRequest, in workdirArgs) (*mcp.CallToolResult, statusOut, error) {
	_ = ctx
	var zero statusOut
	if err := requireIDs(in.TenantID, in.TrajectoryID); err != nil {
		return nil, zero, err
	}
	workDir, err := requireBoundedWorkDir(in.WorkDir)
	if err != nil {
		return nil, zero, err
	}
	nodesPath, _, err := workdirSQLitePaths(workDir)
	if err != nil {
		return nil, zero, err
	}
	nl, err := nodelog.Open(nodesPath)
	if err != nil {
		return nil, zero, err
	}
	defer nl.Close()

	rows, err := nl.ListNodes(in.TrajectoryID, in.TenantID)
	if err != nil {
		return nil, zero, err
	}
	byKind := map[string]int{}
	seals := 0
	for _, r := range rows {
		kind, _ := r["kind"].(string)
		byKind[kind]++
		if kind == "DECISION" {
			seals++
		}
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return nil, statusOut{
		WorkDir:      workDir,
		NodesPath:    nodesPath,
		TenantID:     in.TenantID,
		TrajectoryID: in.TrajectoryID,
		NodeCount:    len(rows),
		SealCount:    seals,
		CountsByKind: byKind,
		Kinds:        kinds,
	}, nil
}

type exportIn struct {
	WorkDir      string `json:"work_dir" jsonschema:"directory under workspace root containing nodes.sqlite"`
	TenantID     string `json:"tenant_id" jsonschema:"tenant id"`
	TrajectoryID string `json:"trajectory_id" jsonschema:"trajectory id"`
	Dest         string `json:"dest" jsonschema:"output .tir path (must stay under work_dir / workspace)"`
	Mode         string `json:"mode,omitempty" jsonschema:"package mode: thin (default) or fat"`
}

type exportOut struct {
	Path         string `json:"path"`
	Mode         string `json:"mode"`
	TrajectoryID string `json:"trajectory_id"`
	TenantID     string `json:"tenant_id"`
	NodeCount    int    `json:"node_count"`
}

func toolExportTIR(ctx context.Context, _ *mcp.CallToolRequest, in exportIn) (*mcp.CallToolResult, exportOut, error) {
	_ = ctx
	var zero exportOut
	if err := requireIDs(in.TenantID, in.TrajectoryID); err != nil {
		return nil, zero, err
	}
	if strings.TrimSpace(in.Dest) == "" {
		return nil, zero, fmt.Errorf("mcp: dest is required")
	}
	mode := tir.ModeThin
	switch strings.ToLower(strings.TrimSpace(in.Mode)) {
	case "", "thin":
		mode = tir.ModeThin
	case "fat":
		mode = tir.ModeFat
	default:
		return nil, zero, fmt.Errorf("mcp: unsupported mode %q (use thin or fat)", in.Mode)
	}

	workDir, err := requireBoundedWorkDir(in.WorkDir)
	if err != nil {
		return nil, zero, err
	}
	dest, err := requireBoundedPath(in.Dest, workDir)
	if err != nil {
		return nil, zero, err
	}
	nodesPath, memoPath, err := workdirSQLitePaths(workDir)
	if err != nil {
		return nil, zero, err
	}

	tr, err := client.OpenTrajectory(in.TenantID, in.TrajectoryID, client.Options{
		NodesPath: nodesPath,
		MemoPath:  memoPath,
	})
	if err != nil {
		return nil, zero, err
	}
	defer tr.Close()

	path, err := tir.Export(tr.Log(), in.TrajectoryID, dest, tir.ExportOptions{
		Mode:     mode,
		TenantID: &in.TenantID,
	})
	if err != nil {
		return nil, zero, err
	}
	rows, err := tr.Log().ListNodes(in.TrajectoryID, in.TenantID)
	if err != nil {
		return nil, zero, err
	}
	return nil, exportOut{
		Path:         path,
		Mode:         string(mode),
		TrajectoryID: in.TrajectoryID,
		TenantID:     in.TenantID,
		NodeCount:    len(rows),
	}, nil
}

type pathIn struct {
	Path string `json:"path" jsonschema:"path to a .tir package under the workspace root"`
}

type importOut struct {
	Path         string `json:"path"`
	Mode         string `json:"mode"`
	TrajectoryID string `json:"trajectory_id"`
	TenantID     string `json:"tenant_id"`
	NodeCount    int    `json:"node_count"`
	SealCount    int    `json:"seal_count"`
	Signed       bool   `json:"signed"`
}

func toolImportTIR(ctx context.Context, _ *mcp.CallToolRequest, in pathIn) (*mcp.CallToolResult, importOut, error) {
	_ = ctx
	var zero importOut
	if strings.TrimSpace(in.Path) == "" {
		return nil, zero, fmt.Errorf("mcp: path is required")
	}
	path, err := requireBoundedPath(in.Path, "")
	if err != nil {
		return nil, zero, err
	}
	pkg, err := tir.Load(path)
	if err != nil {
		return nil, zero, err
	}
	mode, _ := pkg.Manifest["mode"].(string)
	traj, _ := pkg.Manifest["trajectory_id"].(string)
	tenant, _ := pkg.Manifest["tenant_id"].(string)
	return nil, importOut{
		Path:         path,
		Mode:         mode,
		TrajectoryID: traj,
		TenantID:     tenant,
		NodeCount:    len(pkg.Nodes),
		SealCount:    len(pkg.Seals),
		Signed:       pkg.Signature != nil,
	}, nil
}

type verifyIn struct {
	Path             string `json:"path" jsonschema:"path to a .tir package under the workspace root"`
	RequireSignature bool   `json:"require_signature,omitempty" jsonschema:"if true, unsigned packages fail"`
}

type verifyOut struct {
	Path string `json:"path"`
	// Status is unsigned | verified | failed. Unsigned is not the same as verified.
	Status     string `json:"status"`
	Signed     bool   `json:"signed"`
	Verified   bool   `json:"verified"`
	Scheme     string `json:"scheme,omitempty"`
	KeyID      string `json:"key_id,omitempty"`
	SignerID   string `json:"signer_id,omitempty"`
	PayloadHex string `json:"payload_hash_hex,omitempty"`
	Message    string `json:"message"`
}

func toolVerifySignature(ctx context.Context, _ *mcp.CallToolRequest, in verifyIn) (*mcp.CallToolResult, verifyOut, error) {
	_ = ctx
	var zero verifyOut
	if strings.TrimSpace(in.Path) == "" {
		return nil, zero, fmt.Errorf("mcp: path is required")
	}
	path, err := requireBoundedPath(in.Path, "")
	if err != nil {
		return nil, zero, err
	}
	info, err := tir.Verify(path, tir.VerifyOptions{RequireSignature: in.RequireSignature})
	if err != nil {
		if errors.Is(err, tir.ErrSignature) {
			// Signature policy failures (tamper, mismatch, missing-when-required)
			// are a defined tool outcome, not a protocol error: return a
			// structured Status:"failed" result so it reaches the wire, instead
			// of letting the generic MCP error wrapper discard verifyOut.
			return nil, verifyOut{
				Path:     path,
				Status:   "failed",
				Verified: false,
				Message:  err.Error(),
			}, nil
		}
		return nil, zero, err
	}
	if info == nil {
		return nil, verifyOut{
			Path:     path,
			Status:   "unsigned",
			Signed:   false,
			Verified: false,
			Message:  "unsigned package (no SIGNATURE member); crypto verify was not run",
		}, nil
	}
	scheme := ""
	if info.Document != nil {
		scheme = info.Document.Scheme
	}
	return nil, verifyOut{
		Path:       path,
		Status:     "verified",
		Signed:     true,
		Verified:   true,
		Scheme:     scheme,
		KeyID:      info.KeyID,
		SignerID:   info.SignerID,
		PayloadHex: fmt.Sprintf("%x", info.PayloadHash),
		Message:    "signature valid",
	}, nil
}

func requireIDs(tenantID, trajectoryID string) error {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(trajectoryID) == "" {
		return fmt.Errorf("mcp: tenant_id and trajectory_id are required")
	}
	return nil
}
