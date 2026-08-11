// Package client is a thin Go surface over Trajectory IR primitives.
//
// Python mapping (client/python/trajectory_client.py):
//
//	OpenTrajectory  ~ open_trajectory
//	Resume          ~ resume
//	Project         ~ project
//	SealDecision    ~ seal_decision
//	ExecTool        ~ exec_tool
//	CommitStep      ~ commit_step
//	RunStep         ~ full sealed step (resume.RunStep convenience)
package client

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/resume"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
)

// Options configure workdir paths and optional durable backend injection.
type Options struct {
	// WorkDir holds default relative paths for sqlite files when set.
	WorkDir string
	// NodesPath is the NodeLog sqlite path. Default: WorkDir/nodes.sqlite or nodes.sqlite.
	NodesPath string
	// MemoPath is the LocalSQLite durable memo path. Default: WorkDir/memo.sqlite or memo.sqlite.
	MemoPath string
	// WorkflowID scopes durable memo keys. Default: TrajectoryID.
	WorkflowID string
	// Backend overrides LocalSQLite when non nil. Caller owns Close unless Open created it.
	Backend durable.Backend
	// Mode is live (default) or sandbox (R06).
	Mode sandbox.Mode
}

// Trajectory is an open IR run handle.
type Trajectory struct {
	TrajectoryID string
	TenantID     string
	WorkflowID   string
	Mode         sandbox.Mode

	log         *nodelog.NodeLog
	backend     durable.Backend
	ownsBackend bool
	ownsLog     bool
}

// ProjectContext is returned by Project.
type ProjectContext struct {
	StepN   int
	Context map[string]any
}

// Decision is returned by SealDecision.
type Decision struct {
	StepN int
	Plan  map[string]any
}

// ToolResult is returned by ExecTool.
type ToolResult struct {
	StepN  int
	Result any
}

// OpenTrajectory opens or creates log and durable memo stores for a trajectory.
func OpenTrajectory(tenantID, trajectoryID string, opts Options) (*Trajectory, error) {
	if tenantID == "" || trajectoryID == "" {
		return nil, fmt.Errorf("client: tenantID and trajectoryID are required")
	}
	nodesPath, memoPath := resolvePaths(opts)
	wf := opts.WorkflowID
	if wf == "" {
		wf = trajectoryID
	}
	mode := opts.Mode
	if mode == "" {
		mode = sandbox.ModeLive
	}
	if mode != sandbox.ModeLive && mode != sandbox.ModeSandbox {
		return nil, fmt.Errorf("client: unsupported mode %q", mode)
	}

	nl, err := nodelog.Open(nodesPath)
	if err != nil {
		return nil, err
	}

	var backend durable.Backend
	ownsBackend := false
	if opts.Backend != nil {
		backend = opts.Backend
	} else {
		backend, err = durable.OpenLocal(memoPath)
		if err != nil {
			_ = nl.Close()
			return nil, err
		}
		ownsBackend = true
	}

	return &Trajectory{
		TrajectoryID: trajectoryID,
		TenantID:     tenantID,
		WorkflowID:   wf,
		Mode:         mode,
		log:          nl,
		backend:      backend,
		ownsBackend:  ownsBackend,
		ownsLog:      true,
	}, nil
}

// Resume reattaches to a trajectory that already has NodeLog history.
// Unlike OpenTrajectory, empty history is an error (wrong workdir or id).
// Crash safety still comes from NodeLog idempotency and gated tool claims;
// callers re-drive the same step_n/seq work against the reopened log.
func Resume(tenantID, trajectoryID string, opts Options) (*Trajectory, error) {
	nodesPath, _ := resolvePaths(opts)
	tr, err := OpenTrajectory(tenantID, trajectoryID, opts)
	if err != nil {
		return nil, err
	}
	rows, err := tr.log.ListNodesAllTenants(trajectoryID)
	if err != nil {
		_ = tr.Close()
		return nil, err
	}
	if len(rows) == 0 {
		_ = tr.Close()
		return nil, fmt.Errorf(
			"client: cannot resume trajectory_id=%q: no existing nodes found in %q; use OpenTrajectory to start a new one",
			trajectoryID,
			nodesPath,
		)
	}
	return tr, nil
}

// Close releases owned resources.
func (t *Trajectory) Close() error {
	var first error
	if t.ownsLog && t.log != nil {
		if err := t.log.Close(); err != nil && first == nil {
			first = err
		}
		t.log = nil
	}
	if t.ownsBackend && t.backend != nil {
		if err := t.backend.Close(); err != nil && first == nil {
			first = err
		}
		t.backend = nil
	}
	return first
}

// Project appends PROJECT_CONTEXT at seq 0 for the step.
func (t *Trajectory) Project(stepN int, context map[string]any) (*ProjectContext, error) {
	if context == nil {
		context = map[string]any{}
	}
	step := stepN
	if _, err := t.log.Append(
		"PROJECT_CONTEXT",
		&step,
		context,
		t.TrajectoryID,
		t.TenantID,
		0,
	); err != nil {
		return nil, err
	}
	return &ProjectContext{StepN: stepN, Context: context}, nil
}

// SealDecision appends DECISION at seq 1 for the step.
func (t *Trajectory) SealDecision(stepN int, plan map[string]any) (*Decision, error) {
	if plan == nil {
		plan = map[string]any{}
	}
	step := stepN
	if _, err := t.log.Append(
		"DECISION",
		&step,
		map[string]any{"plan": plan},
		t.TrajectoryID,
		t.TenantID,
		1,
	); err != nil {
		return nil, err
	}
	return &Decision{StepN: stepN, Plan: plan}, nil
}

// ExecTool runs one tool at the given seq (caller supplies unique seq; 2+2*i is typical).
// NON_IDEMPOTENT_WRITE tools use claim gate logging. Other effect classes still
// append TOOL_CALL / TOOL_RESULT so the IR history matches the Python client.
func (t *Trajectory) ExecTool(stepN, seq int, tool resume.Tool, args map[string]any) (*ToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	if err := sandbox.AssertToolAllowed(t.Mode, tool.Name, tool.Effect); err != nil {
		return nil, err
	}
	step := stepN
	if effects.RequiresBlockAndGate(tool.Effect) {
		fn := resume.MakeGatedToolCall(
			t.log,
			t.TrajectoryID,
			t.TenantID,
			stepN,
			seq,
			tool.Name,
			tool.Fn,
		)
		result, err := fn(args)
		if err != nil {
			return nil, err
		}
		return &ToolResult{StepN: stepN, Result: result}, nil
	}
	result, err := tool.Fn(args)
	if err != nil {
		return nil, err
	}
	if _, err := t.log.Append(
		"TOOL_CALL",
		&step,
		map[string]any{"tool": tool.Name, "args": args},
		t.TrajectoryID,
		t.TenantID,
		seq,
	); err != nil {
		return nil, err
	}
	if _, err := t.log.Append(
		"TOOL_RESULT",
		&step,
		map[string]any{"result": result},
		t.TrajectoryID,
		t.TenantID,
		seq+1,
	); err != nil {
		return nil, err
	}
	return &ToolResult{StepN: stepN, Result: result}, nil
}

// CommitStep appends COMMIT_STEP at seq (typically 2+2*numTools).
func (t *Trajectory) CommitStep(stepN, seq int) error {
	step := stepN
	_, err := t.log.Append(
		"COMMIT_STEP",
		&step,
		map[string]any{},
		t.TrajectoryID,
		t.TenantID,
		seq,
	)
	return err
}

// RunStepOpts are optional hooks for RunStep (demo and tests).
type RunStepOpts struct {
	OnDecisionSealed func()
}

// RunStep runs a full sealed step via resume.RunStep (project, infer, decision, tools, commit).
func (t *Trajectory) RunStep(
	ctx context.Context,
	stepN int,
	model resume.ModelFunc,
	tools map[string]resume.Tool,
	stepContext map[string]any,
	opts ...RunStepOpts,
) ([]any, error) {
	cfg := resume.RunStepConfig{
		Log:          t.log,
		Backend:      t.backend,
		TenantID:     t.TenantID,
		TrajectoryID: t.TrajectoryID,
		WorkflowID:   t.WorkflowID,
		Tools:        tools,
		Mode:         t.Mode,
	}
	if len(opts) > 0 {
		cfg.OnDecisionSealed = opts[0].OnDecisionSealed
	}
	return resume.RunStep(ctx, cfg, stepN, model, stepContext)
}

// Log exposes the underlying NodeLog for advanced tests.
func (t *Trajectory) Log() *nodelog.NodeLog { return t.log }

// Backend exposes the durable backend for advanced tests.
func (t *Trajectory) Backend() durable.Backend { return t.backend }

func resolvePaths(opts Options) (nodesPath, memoPath string) {
	base := opts.WorkDir
	nodesPath = opts.NodesPath
	memoPath = opts.MemoPath
	if nodesPath == "" {
		if base != "" {
			nodesPath = filepath.Join(base, "nodes.sqlite")
		} else {
			nodesPath = "nodes.sqlite"
		}
	}
	if memoPath == "" {
		if base != "" {
			memoPath = filepath.Join(base, "memo.sqlite")
		} else {
			memoPath = "memo.sqlite"
		}
	}
	return nodesPath, memoPath
}
