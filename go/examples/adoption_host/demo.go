// Package main is the Go adoption host demo (public client APIs only).
//
//	go run ./examples/adoption_host
//	go run ./examples/adoption_host -sandbox
//	go run ./examples/adoption_host -with-package
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/cas"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/client"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/tir"
)

const (
	tenantID     = "demo"
	trajectoryID = "go-adoption-host"
	stepN        = 1
)

// HostStepResult is the outcome of one host owned seal cycle.
type HostStepResult struct {
	ToolResults  []any
	NodeKinds    []string
	TrajectoryID string
	TenantID     string
	WorkDir      string
}

// PackageResult is the outcome of thin export + rehydrate.
type PackageResult struct {
	TirPath      string
	ContentHash  string
	LogicalPath  string
	Rehydrated   []byte
	NodeCount    int
}

func stubModel(ctx map[string]any) map[string]any {
	release := "1.0.0"
	if v, ok := ctx["release"].(string); ok && v != "" {
		release = v
	}
	service := "api"
	if v, ok := ctx["service"].(string); ok && v != "" {
		service = v
	}
	return map[string]any{
		"tool_calls": []any{
			map[string]any{
				"name": "build_manifest",
				"args": map[string]any{"service": service, "release": release},
			},
			map[string]any{
				"name": "ship_release",
				"args": map[string]any{"service": service, "version": release},
			},
		},
	}
}

func makeTools() map[string]resume.Tool {
	return map[string]resume.Tool{
		"build_manifest": {
			Name:   "build_manifest",
			Effect: effects.PURE,
			Fn: func(args map[string]any) (any, error) {
				return map[string]any{
					"service": args["service"],
					"release": args["release"],
					"status":  "ready",
				}, nil
			},
		},
		"ship_release": {
			Name:   "ship_release",
			Effect: effects.NON_IDEMPOTENT_WRITE,
			Fn: func(args map[string]any) (any, error) {
				return map[string]any{
					"shipped": fmt.Sprintf("%v:%v", args["service"], args["version"]),
				}, nil
			},
		},
	}
}

func runHostStep(workDir string, sandboxMode bool, model func(map[string]any) map[string]any) (*HostStepResult, error) {
	if model == nil {
		model = stubModel
	}
	mode := sandbox.ModeLive
	if sandboxMode {
		mode = sandbox.ModeSandbox
	}
	tr, err := client.OpenTrajectory(tenantID, trajectoryID, client.Options{
		WorkDir: workDir,
		Mode:    mode,
	})
	if err != nil {
		return nil, err
	}
	defer tr.Close()

	ctx := map[string]any{
		"goal":    "build a release manifest then ship it",
		"service": "api",
		"release": "1.0.0",
	}
	if _, err := tr.Project(stepN, ctx); err != nil {
		return nil, err
	}
	plan := model(ctx)
	if _, err := tr.SealDecision(stepN, plan); err != nil {
		return nil, err
	}

	tools := makeTools()
	rawCalls, ok := plan["tool_calls"].([]any)
	if !ok {
		return nil, fmt.Errorf("model plan must include tool_calls")
	}
	results := make([]any, 0, len(rawCalls))
	for i, raw := range rawCalls {
		call, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tool call %d is not an object", i)
		}
		name, _ := call["name"].(string)
		tool, ok := tools[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool in sealed plan: %q", name)
		}
		args, _ := call["args"].(map[string]any)
		if args == nil {
			args = map[string]any{}
		}
		seq := 2 + 2*i
		trRes, err := tr.ExecTool(stepN, seq, tool, args)
		if err != nil {
			return nil, err
		}
		results = append(results, trRes.Result)
	}
	commitSeq := 2 + 2*len(rawCalls)
	if err := tr.CommitStep(stepN, commitSeq); err != nil {
		return nil, err
	}

	kinds, err := listKinds(tr, trajectoryID, tenantID)
	if err != nil {
		return nil, err
	}
	return &HostStepResult{
		ToolResults:  results,
		NodeKinds:    kinds,
		TrajectoryID: trajectoryID,
		TenantID:     tenantID,
		WorkDir:      workDir,
	}, nil
}

func listKinds(tr *client.Trajectory, trajID, tenant string) ([]string, error) {
	rows, err := tr.Log().ListNodes(trajID, tenant)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if k, ok := r["kind"].(string); ok {
			out = append(out, k)
		}
	}
	return out, nil
}

func reportBytes(step *HostStepResult) ([]byte, error) {
	body := map[string]any{
		"trajectory_id": step.TrajectoryID,
		"tenant_id":     step.TenantID,
		"tool_results":  step.ToolResults,
		"node_kinds":    step.NodeKinds,
	}
	return json.MarshalIndent(body, "", "  ")
}

func exportThinPackage(workDir, casRoot, dest string, payload []byte) (*PackageResult, error) {
	if len(payload) == 0 {
		return nil, errors.New("payload must be non empty")
	}
	store, err := cas.NewFileSystem(casRoot)
	if err != nil {
		return nil, err
	}
	h, err := store.Put(payload)
	if err != nil {
		return nil, err
	}
	uri, err := store.URIFor(h)
	if err != nil {
		return nil, err
	}
	size := len(payload)
	logical := "outputs/release-report.json"
	ref := tir.ArtifactRef{
		LogicalPath: logical,
		ContentHash: h,
		URI:         &uri,
		Size:        &size,
	}

	tr, err := client.OpenTrajectory(tenantID, trajectoryID, client.Options{WorkDir: workDir})
	if err != nil {
		return nil, err
	}
	defer tr.Close()
	tenant := tenantID
	path, err := tir.Export(tr.Log(), trajectoryID, dest, tir.ExportOptions{
		Mode:      tir.ModeThin,
		TenantID:  &tenant,
		Artifacts: []tir.ArtifactRef{ref},
	})
	if err != nil {
		return nil, err
	}
	pkg, err := tir.Load(path)
	if err != nil {
		return nil, err
	}
	entries := make([]cas.ArtifactEntry, 0, len(pkg.ArtifactsManifest))
	for _, m := range pkg.ArtifactsManifest {
		e := cas.ArtifactEntry{}
		if s, ok := m["content_hash"].(string); ok {
			e.ContentHash = s
		}
		if s, ok := m["logical_path"].(string); ok {
			e.LogicalPath = s
		}
		entries = append(entries, e)
	}
	got, err := cas.Rehydrate(store, entries, true)
	if err != nil {
		return nil, err
	}
	data, ok := got[h]
	if !ok {
		return nil, fmt.Errorf("rehydrate missed content_hash=%s", h)
	}
	nodeCount := 0
	if n, ok := pkg.Manifest["node_count"].(float64); ok {
		nodeCount = int(n)
	} else if n, ok := pkg.Manifest["node_count"].(int); ok {
		nodeCount = n
	}
	return &PackageResult{
		TirPath:     path,
		ContentHash: h,
		LogicalPath: logical,
		Rehydrated:  data,
		NodeCount:   nodeCount,
	}, nil
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func defaultWorkDir() (string, error) {
	return os.MkdirTemp("", "trajir-adoption-*")
}

func absJoin(dir, name string) string {
	return filepath.Join(dir, name)
}
