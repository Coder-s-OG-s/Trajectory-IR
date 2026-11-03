// Package resume holds Go seal and resume helpers: block and gate, and RunStep.
package resume

import (
	"fmt"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
)

const integerDomain = "integer_domain"

// BlockedNeedsGate is raised when a NON_IDEMPOTENT_WRITE tool was already
// attempted for this step/seq and must not be silently re-run.
//
// This is at-most-one automatic attempt from this client, not exactly-once
// in the world. A human or host policy must confirm success, confirm the
// call never landed, compensate, or re-run in a new step (spec §8.3).
type BlockedNeedsGate struct {
	StepN    int
	ToolName string
}

func (e *BlockedNeedsGate) Error() string {
	return fmt.Sprintf(
		"step %d: '%s' BLOCKED_NEEDS_GATE (crashed mid-execution, not retried)",
		e.StepN,
		e.ToolName,
	)
}

// ToolCallConflict is returned when re-entry reaches a claimed TOOL_CALL slot
// with a different tool name or args. The stored outcome belongs to the call
// that claimed the slot, so it is not handed back, and the new call does not
// run. errors.Is(err, nodelog.ErrSlotConflict) is true.
type ToolCallConflict struct {
	StepN    int
	Seq      int
	ToolName string
}

func (e *ToolCallConflict) Error() string {
	return fmt.Sprintf(
		"step %d seq %d: '%s' does not match the TOOL_CALL already claimed at this slot (different tool or args)",
		e.StepN,
		e.Seq,
		e.ToolName,
	)
}

func (e *ToolCallConflict) Unwrap() error { return nodelog.ErrSlotConflict }

// UnsafeInteger is returned when the TOOL_RESULT at this slot is the
// integer_domain storage failure the Python client records (spec §6.2).
// It is not a result and not ABORT. The tool body does not run again.
type UnsafeInteger struct {
	Message string
}

func (e *UnsafeInteger) Error() string { return e.Message }

// ToolFunc is a tool body that receives a free-form args map.
type ToolFunc func(args map[string]any) (any, error)

// ResolveFunc looks up how a claimed call with no recorded outcome ended, for
// example a peer agent (AGENT_SPAWN) still running on its own durable backend.
// It gets the call's args and CallMeta so it can query by idempotency key. It
// must not start the effect again. resolved=false means the host cannot tell,
// and the gate blocks.
type ResolveFunc func(args map[string]any, meta CallMeta) (result any, resolved bool, err error)

// MakeGatedToolCall wraps toolFn so re-entry after TOOL_CALL was claimed blocks,
// unless a TOOL_RESULT was already recorded, in which case that result is returned.
//
// Seq layout matches Python: TOOL_CALL at seq, TOOL_RESULT or ABORT at seq+1.
// Callers should space tools as seq = 2 + 2*i for tool index i inside a step.
//
// The claim is atomic (transactional insert). Concurrent workers cannot both
// win the same (trajectory, step, seq) TOOL_CALL slot.
//
// Re-entry must carry the same tool name and args as the claimed TOOL_CALL,
// or it returns ToolCallConflict.
func MakeGatedToolCall(
	log *nodelog.NodeLog,
	trajectoryID, tenantID string,
	stepN, seq int,
	toolName string,
	toolFn ToolFunc,
) ToolFunc {
	return MakeGatedToolCallWithResolve(log, trajectoryID, tenantID, stepN, seq, toolName, toolFn, nil)
}

// MakeGatedToolCallWithResolve is MakeGatedToolCall with a status lookup.
// When the slot is claimed and has no recorded outcome, resolve is asked
// before blocking. A resolved result is recorded as the TOOL_RESULT and
// returned. Unresolved falls through to ABORT and BlockedNeedsGate. resolve
// is asked again on each re-entry, including after an earlier ABORT here.
// A nil resolve behaves like MakeGatedToolCall.
func MakeGatedToolCallWithResolve(
	log *nodelog.NodeLog,
	trajectoryID, tenantID string,
	stepN, seq int,
	toolName string,
	toolFn ToolFunc,
	resolve ResolveFunc,
) ToolFunc {
	return func(args map[string]any) (any, error) {
		if args == nil {
			args = map[string]any{}
		}
		step := stepN

		claimed, err := log.ClaimToolCall(
			stepN,
			map[string]any{
				"tool":            toolName,
				"args":            args,
				"idempotency_key": IdempotencyKey(tenantID, trajectoryID, stepN, seq),
			},
			trajectoryID,
			tenantID,
			seq,
		)
		if err != nil {
			return nil, err
		}
		if !claimed {
			if err := assertSameCall(log, trajectoryID, tenantID, stepN, seq, toolName, args); err != nil {
				return nil, err
			}
			if err := storedIntegerFailure(log, trajectoryID, tenantID, stepN, seq+1); err != nil {
				return nil, err
			}
			// The effect already ran and its outcome is durable. Return it
			// instead of blocking; nothing re-executes.
			stored, found, err := storedToolResult(log, trajectoryID, tenantID, stepN, seq+1)
			if err != nil {
				return nil, err
			}
			if found {
				return stored, nil
			}
			if resolve != nil {
				result, resolved, err := resolve(args, NewCallMeta(tenantID, trajectoryID, stepN, seq))
				if err != nil {
					return nil, err
				}
				if resolved {
					if err := appendToolResult(log, trajectoryID, tenantID, stepN, seq+1, result); err != nil {
						return nil, err
					}
					return result, nil
				}
			}
			if _, err := log.Append(
				"ABORT",
				&step,
				map[string]any{"reason": "BLOCKED_NEEDS_GATE", "tool": toolName},
				trajectoryID,
				tenantID,
				seq+1,
			); err != nil {
				return nil, err
			}
			return nil, &BlockedNeedsGate{StepN: stepN, ToolName: toolName}
		}

		result, err := toolFn(args)
		if err != nil {
			return nil, err
		}
		if err := appendToolResult(log, trajectoryID, tenantID, stepN, seq+1, result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

func appendToolResult(log *nodelog.NodeLog, trajectoryID, tenantID string, stepN, seq int, result any) error {
	step := stepN
	_, err := log.Append(
		"TOOL_RESULT",
		&step,
		map[string]any{"result": result},
		trajectoryID,
		tenantID,
		seq,
	)
	return err
}

// assertSameCall returns ToolCallConflict unless the claimed TOOL_CALL at
// (stepN, seq) has this tool name and args. Compared by RFC 8785 hash, so
// int 1 and float64 1 match the way they would in the node id.
func assertSameCall(
	log *nodelog.NodeLog,
	trajectoryID, tenantID string,
	stepN, seq int,
	toolName string,
	args map[string]any,
) error {
	stored, found, err := log.GetNodePayload(trajectoryID, tenantID, stepN, seq, "TOOL_CALL")
	if err != nil {
		return err
	}
	conflict := &ToolCallConflict{StepN: stepN, Seq: seq, ToolName: toolName}
	if !found {
		return conflict
	}
	storedArgs := stored["args"]
	if storedArgs == nil {
		storedArgs = map[string]any{}
	}
	want, err := nodes.PayloadHash(map[string]any{"tool": stored["tool"], "args": storedArgs})
	if err != nil {
		return err
	}
	got, err := nodes.PayloadHash(map[string]any{"tool": toolName, "args": args})
	if err != nil {
		return err
	}
	if want != got {
		return conflict
	}
	return nil
}

// storedIntegerFailure returns UnsafeInteger when the TOOL_RESULT at
// (stepN, seq) is an integer_domain storage failure.
func storedIntegerFailure(
	log *nodelog.NodeLog,
	trajectoryID, tenantID string,
	stepN, seq int,
) error {
	payload, found, err := log.GetNodePayload(trajectoryID, tenantID, stepN, seq, "TOOL_RESULT")
	if err != nil || !found {
		return err
	}
	if payload["error"] != integerDomain {
		return nil
	}
	if _, ok := payload["result_digest"].(string); !ok {
		return nil
	}
	message, _ := payload["message"].(string)
	if message == "" {
		message = "integer exceeds safe integer domain for JSON floats"
	}
	return &UnsafeInteger{Message: message}
}

// storedToolResult returns the result recorded in the TOOL_RESULT node at
// (stepN, seq), if there is one.
func storedToolResult(
	log *nodelog.NodeLog,
	trajectoryID, tenantID string,
	stepN, seq int,
) (any, bool, error) {
	payload, found, err := log.GetNodePayload(trajectoryID, tenantID, stepN, seq, "TOOL_RESULT")
	if err != nil || !found {
		return nil, false, err
	}
	result, ok := payload["result"]
	return result, ok, nil
}

// MakePlainToolCall wraps toolFn for non-gated tools with audit logging.
//
// Seq layout matches Python: TOOL_CALL at seq, TOOL_RESULT at seq+1.
// TOOL_CALL is recorded prior to tool execution so failed attempts are audited.
// On success, TOOL_RESULT is recorded at seq+1 with {"result": ...}. An
// integer_domain TOOL_RESULT already at seq+1 returns UnsafeInteger and the
// body does not run, matching Python.
func MakePlainToolCall(
	log *nodelog.NodeLog,
	trajectoryID, tenantID string,
	stepN, seq int,
	toolName string,
	toolFn ToolFunc,
) ToolFunc {
	return func(args map[string]any) (any, error) {
		if args == nil {
			args = map[string]any{}
		}
		step := stepN

		if err := storedIntegerFailure(log, trajectoryID, tenantID, stepN, seq+1); err != nil {
			return nil, err
		}
		if _, err := log.Append(
			"TOOL_CALL",
			&step,
			map[string]any{
				"tool":            toolName,
				"args":            args,
				"idempotency_key": IdempotencyKey(tenantID, trajectoryID, stepN, seq),
			},
			trajectoryID,
			tenantID,
			seq,
		); err != nil {
			return nil, err
		}

		result, err := toolFn(args)
		if err != nil {
			return nil, err
		}
		if err := appendToolResult(log, trajectoryID, tenantID, stepN, seq+1, result); err != nil {
			return nil, err
		}
		return result, nil
	}
}
