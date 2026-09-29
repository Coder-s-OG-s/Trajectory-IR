package resume

import (
	"context"
	"fmt"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/durable"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/effects"
	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/sandbox"
)

// Tool is a named tool with an effect class and implementation.
type Tool struct {
	Name   string
	Effect effects.EffectClass
	Fn     ToolFunc
	// FnWithMeta, when set, is preferred over Fn so the tool can forward
	// the hashed idempotency key without it being stuffed into args.
	FnWithMeta ToolFuncWithMeta
	// AllowOpenWorldOverride is required to tag bash/python/sql/browser as
	// PURE, READ_ONLY, or IDEMPOTENT_WRITE. Without it, ExecTool / RunStep
	// refuse the claim.
	AllowOpenWorldOverride bool
}

// ToolFuncWithMeta is a tool body that also receives CallMeta.
type ToolFuncWithMeta func(args map[string]any, meta CallMeta) (any, error)

// BoundToolFn returns the function MakeGatedToolCall / MakePlainToolCall run.
// FnWithMeta wins when set so the key can leave the process via headers.
func BoundToolFn(tool Tool, meta CallMeta) ToolFunc {
	if tool.FnWithMeta != nil {
		return func(args map[string]any) (any, error) {
			return tool.FnWithMeta(args, meta)
		}
	}
	if tool.Fn != nil {
		return tool.Fn
	}
	return func(map[string]any) (any, error) {
		return nil, fmt.Errorf("resume: tool %q has no function", tool.Name)
	}
}

// ModelFunc produces a plan map for a step. Plan must include "tool_calls":
// a list of maps with "name" and optional "args".
type ModelFunc func(ctx context.Context, context map[string]any) (map[string]any, error)

// RunStepConfig wires IR log, durable memo backend, and tools for one trajectory.
type RunStepConfig struct {
	Log              *nodelog.NodeLog
	Backend          durable.Backend
	TenantID         string
	TrajectoryID     string
	WorkflowID       string // durable memo scope; often same as TrajectoryID
	Tools            map[string]Tool
	OnDecisionSealed func() // optional test hook after DECISION is appended
	// Mode is live (default) or sandbox (R06: reject NON_IDEMPOTENT_WRITE, AGENT_SPAWN, SENSITIVE).
	Mode sandbox.Mode
	// WorldSnapshot is sealed onto DECISION when non-empty (spec §8.4).
	WorldSnapshot map[string]string
	// ObserveWorld, when set, is called after the seal exists (including on
	// resume) and compared with CheckWorld. Empty sealed snapshot is a no-op.
	ObserveWorld func() (map[string]string, error)
}

// RunStep executes one agent step: project context, durable infer, DECISION seal,
// tools (gated when RequiresBlockAndGate), then COMMIT_STEP.
//
// Resume matrix (README §8, R02 / R03): NON_IDEMPOTENT_WRITE is block-and-gated;
// PURE and other non-gated classes may recompute on resume without BlockedNeedsGate.
//
// Seq layout matches Python make_run_step:
//
//	0 PROJECT_CONTEXT, 1 DECISION, tools at 2+2*i / 3+2*i, COMMIT at 2+2*n.
func RunStep(
	ctx context.Context,
	cfg RunStepConfig,
	stepN int,
	model ModelFunc,
	stepContext map[string]any,
) ([]any, error) {
	if cfg.Log == nil || cfg.Backend == nil {
		return nil, fmt.Errorf("resume: Log and Backend are required")
	}
	if cfg.WorkflowID == "" {
		cfg.WorkflowID = cfg.TrajectoryID
	}
	if cfg.Mode == "" {
		cfg.Mode = sandbox.ModeLive
	}
	if stepContext == nil {
		stepContext = map[string]any{}
	}
	step := stepN

	if _, err := cfg.Log.Append(
		"PROJECT_CONTEXT",
		&step,
		stepContext,
		cfg.TrajectoryID,
		cfg.TenantID,
		0,
	); err != nil {
		return nil, err
	}

	planAny, err := durable.Infer(ctx, cfg.Backend, cfg.WorkflowID, fmt.Sprintf("step%d", stepN), func(context.Context) (any, error) {
		return model(ctx, stepContext)
	})
	if err != nil {
		return nil, err
	}
	plan, err := asStringMap(planAny)
	if err != nil {
		return nil, fmt.Errorf("resume: plan: %w", err)
	}

	if _, err := cfg.Log.Append(
		"DECISION",
		&step,
		DecisionPayload(plan, cfg.WorldSnapshot),
		cfg.TrajectoryID,
		cfg.TenantID,
		1,
	); err != nil {
		return nil, err
	}
	if cfg.OnDecisionSealed != nil {
		cfg.OnDecisionSealed()
	}
	if cfg.ObserveWorld != nil {
		observed, err := cfg.ObserveWorld()
		if err != nil {
			return nil, err
		}
		if err := CheckWorld(cfg.Log, cfg.TrajectoryID, cfg.TenantID, stepN, observed); err != nil {
			return nil, err
		}
	}

	calls, err := toolCallsFromPlan(plan)
	if err != nil {
		return nil, err
	}

	results := make([]any, 0, len(calls))
	for i, call := range calls {
		tool, ok := cfg.Tools[call.Name]
		if !ok {
			return nil, fmt.Errorf("resume: unknown tool %q", call.Name)
		}
		seq := 2 + 2*i
		args := call.Args
		if args == nil {
			args = map[string]any{}
		}
		if err := effects.AssertOpenWorldEffect(tool.Name, tool.Effect, tool.AllowOpenWorldOverride); err != nil {
			return nil, err
		}
		if err := effects.AssertOpenWorldEffect(call.Name, tool.Effect, tool.AllowOpenWorldOverride); err != nil {
			return nil, err
		}
		if err := sandbox.AssertToolAllowed(cfg.Mode, call.Name, tool.Effect); err != nil {
			return nil, err
		}

		meta := NewCallMeta(cfg.TenantID, cfg.TrajectoryID, stepN, seq)
		fn := BoundToolFn(tool, meta)

		var result any
		stepKey := fmt.Sprintf("%s@%d", call.Name, seq)
		if effects.RequiresBlockAndGate(tool.Effect) {
			gated := MakeGatedToolCall(
				cfg.Log,
				cfg.TrajectoryID,
				cfg.TenantID,
				stepN,
				seq,
				call.Name,
				fn,
			)
			result, err = durable.Tool(ctx, cfg.Backend, cfg.WorkflowID, stepKey, func(context.Context) (any, error) {
				return gated(args)
			})
			if err != nil {
				return nil, err
			}
		} else {
			plain := MakePlainToolCall(
				cfg.Log,
				cfg.TrajectoryID,
				cfg.TenantID,
				stepN,
				seq,
				call.Name,
				fn,
			)
			result, err = durable.Tool(ctx, cfg.Backend, cfg.WorkflowID, stepKey, func(context.Context) (any, error) {
				return plain(args)
			})
			if err != nil {
				return nil, err
			}
		}
		results = append(results, result)
	}

	commitSeq := 2 + 2*len(calls)
	if _, err := cfg.Log.Append(
		"COMMIT_STEP",
		&step,
		map[string]any{},
		cfg.TrajectoryID,
		cfg.TenantID,
		commitSeq,
	); err != nil {
		return nil, err
	}
	return results, nil
}

type toolCall struct {
	Name string
	Args map[string]any
}

func toolCallsFromPlan(plan map[string]any) ([]toolCall, error) {
	raw, ok := plan["tool_calls"]
	if !ok {
		return nil, fmt.Errorf("resume: plan missing tool_calls")
	}
	list, ok := raw.([]any)
	if !ok {
		// JSON may decode as []map under some paths; accept []any only after Infer.
		return nil, fmt.Errorf("resume: tool_calls must be a list")
	}
	out := make([]toolCall, 0, len(list))
	for i, item := range list {
		m, err := asStringMap(item)
		if err != nil {
			return nil, fmt.Errorf("resume: tool_calls[%d]: %w", i, err)
		}
		name, _ := m["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("resume: tool_calls[%d] missing name", i)
		}
		var args map[string]any
		if a, ok := m["args"]; ok && a != nil {
			args, err = asStringMap(a)
			if err != nil {
				return nil, fmt.Errorf("resume: tool_calls[%d].args: %w", i, err)
			}
		}
		out = append(out, toolCall{Name: name, Args: args})
	}
	return out, nil
}

func asStringMap(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	return nil, fmt.Errorf("want object, got %T", v)
}
