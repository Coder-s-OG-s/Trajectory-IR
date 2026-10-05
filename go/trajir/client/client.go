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
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
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
	// Mode is live (default) or sandbox (R06: reject NON_IDEMPOTENT_WRITE, AGENT_SPAWN, SENSITIVE).
	Mode sandbox.Mode
	// ConsoleSink receives console events after a successful append.
	// Nil skips telemetry. Sink errors are logged and do not fail the call.
	ConsoleSink emit.Sink
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
	sink        emit.Sink
}

// ProjectContext is returned by Project.
type ProjectContext struct {
	StepN   int
	Context map[string]any
}

// Decision is returned by SealDecision.
type Decision struct {
	StepN         int
	Plan          map[string]any
	WorldSnapshot map[string]string
}

// SealDecisionOpts is optional input for SealDecision.
type SealDecisionOpts struct {
	WorldSnapshot map[string]string
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
		sink:         opts.ConsoleSink,
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
	rows, err := tr.log.ListNodes(trajectoryID, tenantID)
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
	node, err := t.log.Append(
		"PROJECT_CONTEXT",
		&step,
		context,
		t.TrajectoryID,
		t.TenantID,
		0,
	)
	if err != nil {
		return nil, err
	}
	t.emitNode(node)
	return &ProjectContext{StepN: stepN, Context: context}, nil
}

// SealDecision appends DECISION at seq 1 for the step.
// Pass SealDecisionOpts.WorldSnapshot to opt into fail-loud CheckWorld.
func (t *Trajectory) SealDecision(stepN int, plan map[string]any, opts ...SealDecisionOpts) (*Decision, error) {
	if plan == nil {
		plan = map[string]any{}
	}
	var snap map[string]string
	if len(opts) > 0 {
		snap = opts[0].WorldSnapshot
	}
	step := stepN
	node, err := t.log.Append(
		"DECISION",
		&step,
		resume.DecisionPayload(plan, snap),
		t.TrajectoryID,
		t.TenantID,
		1,
	)
	if err != nil {
		return nil, err
	}
	t.emitNode(node)
	t.emitSeal(node, stepN, plan)
	return &Decision{StepN: stepN, Plan: plan, WorldSnapshot: resume.NormalizeWorldSnapshot(snap)}, nil
}

// CheckWorld compares observed values against a sealed WORLD_SNAPSHOT.
// No-op when the DECISION has no snapshot. Error if this step has no DECISION.
func (t *Trajectory) CheckWorld(stepN int, observed map[string]string) error {
	return resume.CheckWorld(t.log, t.TrajectoryID, t.TenantID, stepN, observed)
}

// ExecTool runs one tool at the given seq (caller supplies unique seq; 2+2*i is typical).
// NON_IDEMPOTENT_WRITE tools use claim gate logging. Other effect classes still
// append TOOL_CALL / TOOL_RESULT so the IR history matches the Python client.
func (t *Trajectory) ExecTool(stepN, seq int, tool resume.Tool, args map[string]any) (*ToolResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	if err := effects.AssertOpenWorldEffect(tool.Name, tool.Effect, tool.AllowOpenWorldOverride); err != nil {
		return nil, err
	}
	if err := sandbox.AssertToolAllowed(t.Mode, tool.Name, tool.Effect); err != nil {
		return nil, err
	}
	meta := resume.NewCallMeta(t.TenantID, t.TrajectoryID, stepN, seq)
	bound := resume.BoundToolFn(tool, meta)
	defer t.emitToolConsole(stepN, seq, tool.Effect)
	if effects.RequiresBlockAndGate(tool.Effect) {
		fn := resume.MakeGatedToolCall(
			t.log,
			t.TrajectoryID,
			t.TenantID,
			stepN,
			seq,
			tool.Name,
			bound,
		)
		result, err := fn(args)
		if err != nil {
			return nil, err
		}
		return &ToolResult{StepN: stepN, Result: result}, nil
	}
	fn := resume.MakePlainToolCall(
		t.log,
		t.TrajectoryID,
		t.TenantID,
		stepN,
		seq,
		tool.Name,
		bound,
	)
	result, err := fn(args)
	if err != nil {
		return nil, err
	}
	return &ToolResult{StepN: stepN, Result: result}, nil
}

// CommitStep appends COMMIT_STEP at seq (typically 2+2*numTools).
func (t *Trajectory) CommitStep(stepN, seq int) error {
	step := stepN
	node, err := t.log.Append(
		"COMMIT_STEP",
		&step,
		map[string]any{},
		t.TrajectoryID,
		t.TenantID,
		seq,
	)
	if err != nil {
		return err
	}
	t.emitNode(node)
	return nil
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

func (t *Trajectory) emitNode(n *nodes.Node) {
	if t == nil || n == nil {
		return
	}
	payload := map[string]any{
		"node_id":      n.ID,
		"kind":         n.Kind,
		"seq":          n.Seq,
		"content_hash": n.PHash,
	}
	if n.StepN != nil {
		payload["step_n"] = *n.StepN
	}
	if n.Kind == "TOOL_CALL" && n.Payload != nil {
		if tool, ok := n.Payload["tool"].(string); ok && tool != "" {
			payload["tool"] = tool
		}
		if key, ok := n.Payload["idempotency_key"].(string); ok && key != "" {
			payload["idempotency_key"] = key
		}
		if eff, ok := n.Payload["effect_class"].(string); ok && eff != "" {
			payload["effect_class"] = eff
		}
	}
	emit.SafeEmit(t.sink, emit.Event{
		Kind:         emit.KindNodeAppended,
		Source:       "go",
		Runtime:      "go",
		TrajectoryID: t.TrajectoryID,
		TenantID:     t.TenantID,
		Payload:      payload,
	})
}

// emitToolConsole emits TOOL_CALL / TOOL_RESULT / ABORT observations for one seq pair.
func (t *Trajectory) emitToolConsole(stepN, seq int, effect effects.EffectClass) {
	if t == nil || t.sink == nil {
		return
	}
	rows, err := t.log.ListNodes(t.TrajectoryID, t.TenantID)
	if err != nil {
		return
	}
	for _, row := range rows {
		kind, _ := row["kind"].(string)
		rowSeq, _ := row["seq"].(int)
		var rowStep int
		switch v := row["step_n"].(type) {
		case int:
			rowStep = v
		case int64:
			rowStep = int(v)
		case float64:
			rowStep = int(v)
		default:
			continue
		}
		if rowStep != stepN || (rowSeq != seq && rowSeq != seq+1) {
			continue
		}
		id, _ := row["id"].(string)
		body, _ := row["payload"].(map[string]any)
		if body == nil {
			body = map[string]any{}
		}
		phash, _ := nodes.PayloadHash(body)
		n := &nodes.Node{
			Kind:         kind,
			TrajectoryID: t.TrajectoryID,
			TenantID:     t.TenantID,
			StepN:        &stepN,
			Seq:          rowSeq,
			Payload:      body,
			PHash:        phash,
			ID:           id,
		}
		if kind == "TOOL_CALL" && effect != "" {
			// Observation-only: effect class is host knowledge, not always on the IR node.
			n.Payload = map[string]any{}
			for k, v := range body {
				n.Payload[k] = v
			}
			n.Payload["effect_class"] = string(effect)
		}
		t.emitNode(n)
	}
}

func (t *Trajectory) emitSeal(n *nodes.Node, stepN int, plan map[string]any) {
	if t == nil || n == nil {
		return
	}
	payload := map[string]any{
		"node_id":      n.ID,
		"step_n":       stepN,
		"content_hash": n.PHash,
	}
	if names := toolNames(plan); len(names) > 0 {
		payload["tool_names"] = names
	}
	emit.SafeEmit(t.sink, emit.Event{
		Kind:         emit.KindSealCreated,
		Source:       "go",
		Runtime:      "go",
		TrajectoryID: t.TrajectoryID,
		TenantID:     t.TenantID,
		Payload:      payload,
	})
}

func toolNames(plan map[string]any) []string {
	raw, ok := plan["tool_calls"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, item := range raw {
		call, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := call["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

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
