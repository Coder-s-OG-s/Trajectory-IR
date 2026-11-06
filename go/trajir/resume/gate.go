// Package resume holds Go seal and resume helpers: block and gate, and RunStep.
package resume

import (
	"fmt"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
)

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

const integerDomain = "integer_domain"

const defaultUnsafeIntegerMessage = "integer exceeds safe integer domain for JSON floats"

// UnsafeInteger is a stored integer_domain TOOL_RESULT. A peer writer
// refused to store an integer outside the RFC 8785 range, so the number
// is not in the log. Resume reports that storage failure. It does not
// run the tool again and it does not write ABORT.
type UnsafeInteger struct {
	Message string
}

func (e *UnsafeInteger) Error() string {
	if e == nil || e.Message == "" {
		return defaultUnsafeIntegerMessage
	}
	return e.Message
}

// ToolFunc is a tool body that receives a free-form args map.
type ToolFunc func(args map[string]any) (any, error)

// MakeGatedToolCall wraps toolFn so re-entry after TOOL_CALL was claimed blocks,
// unless a TOOL_RESULT was already recorded, in which case that result is returned.
//
// Seq layout matches Python: TOOL_CALL at seq, TOOL_RESULT or ABORT at seq+1.
// Callers should space tools as seq = 2 + 2*i for tool index i inside a step.
//
// The claim is atomic (transactional insert). Concurrent workers cannot both
// win the same (trajectory, step, seq) TOOL_CALL slot.
func MakeGatedToolCall(
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

		// Look up seq+1 once, and only when that slot exists. A stored
		// integer_domain row is a storage failure, including when the
		// TOOL_CALL was never written, so the claim must not run the tool.
		payload, found, err := storedToolPayload(log, trajectoryID, tenantID, stepN, seq+1)
		if err != nil {
			return nil, err
		}
		if failed := integerFailure(payload); failed != nil {
			return nil, failed
		}

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
			if !found {
				payload, found, err = storedToolPayload(log, trajectoryID, tenantID, stepN, seq+1)
				if err != nil {
					return nil, err
				}
				if failed := integerFailure(payload); failed != nil {
					return nil, failed
				}
			}
			// The effect already ran and its outcome is durable. Return it
			// instead of blocking; nothing re-executes.
			if found {
				if result, ok := payload["result"]; ok {
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

		if _, err := log.Append(
			"TOOL_RESULT",
			&step,
			map[string]any{"result": result},
			trajectoryID,
			tenantID,
			seq+1,
		); err != nil {
			return nil, err
		}
		return result, nil
	}
}

// storedToolPayload returns the TOOL_RESULT payload at (stepN, seq).
// Has runs first, matching Python, so a first call does not decode the log.
// Callers branch on that one payload: integer_domain, a stored result, or neither.
func storedToolPayload(
	log *nodelog.NodeLog,
	trajectoryID, tenantID string,
	stepN, seq int,
) (map[string]any, bool, error) {
	slot := seq
	has, err := log.Has(trajectoryID, tenantID, stepN, "TOOL_RESULT", &slot)
	if err != nil || !has {
		return nil, false, err
	}
	rows, err := log.ListNodes(trajectoryID, tenantID)
	if err != nil {
		return nil, false, err
	}
	for _, row := range rows {
		if row["kind"] != "TOOL_RESULT" || row["step_n"] != stepN || row["seq"] != seq {
			continue
		}
		payload, ok := row["payload"].(map[string]any)
		if !ok {
			return nil, false, nil
		}
		return payload, true, nil
	}
	return nil, false, nil
}

// integerFailure matches the Python reader. error must be integer_domain
// and result_digest must be a string. Any string counts, including a short
// one. A missing or non-string digest is not this failure, so the caller
// still gates. An empty message uses the default.
func integerFailure(payload map[string]any) *UnsafeInteger {
	if payload == nil || payload["error"] != integerDomain {
		return nil
	}
	if _, ok := payload["result_digest"].(string); !ok {
		return nil
	}
	message, _ := payload["message"].(string)
	if message == "" {
		message = defaultUnsafeIntegerMessage
	}
	return &UnsafeInteger{Message: message}
}

// MakePlainToolCall wraps toolFn for non-gated tools with audit logging.
//
// Seq layout matches Python: TOOL_CALL at seq, TOOL_RESULT at seq+1.
// TOOL_CALL is recorded prior to tool execution so failed attempts are audited.
// On success, TOOL_RESULT is recorded at seq+1 with {"result": ...}.
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

		payload, _, err := storedToolPayload(log, trajectoryID, tenantID, stepN, seq+1)
		if err != nil {
			return nil, err
		}
		if failed := integerFailure(payload); failed != nil {
			return nil, failed
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

		if _, err := log.Append(
			"TOOL_RESULT",
			&step,
			map[string]any{"result": result},
			trajectoryID,
			tenantID,
			seq+1,
		); err != nil {
			return nil, err
		}
		return result, nil
	}
}
