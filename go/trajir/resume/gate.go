// Package resume holds Go seal and resume helpers: block and gate, and RunStep.
package resume

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

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

// UnsafeInteger is returned when a tool result has an integer outside the
// RFC 8785 range, and on re-entry when the TOOL_RESULT at this slot is the
// integer_domain storage failure recorded for it (spec §6.2). It is not a
// result and not ABORT. The tool body does not run again.
type UnsafeInteger = nodes.UnsafeIntegerError

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
// returned. Unresolved returns BlockedNeedsGate without writing ABORT, because
// the call may still finish and one slot holds one outcome. resolve is asked
// again on the next re-entry. An ABORT already at this slot is final: resolve
// is not asked and the call blocks. A nil resolve behaves like MakeGatedToolCall.
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
		meta := NewCallMeta(tenantID, trajectoryID, stepN, seq)

		claimed, err := log.ClaimToolCall(
			stepN,
			map[string]any{
				"tool":            toolName,
				"args":            args,
				"idempotency_key": meta.IdempotencyKey,
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
			_, aborted, err := log.GetNodePayload(trajectoryID, tenantID, &step, seq+1, "ABORT")
			if err != nil {
				return nil, err
			}
			if resolve != nil && !aborted {
				result, resolved, err := resolve(args, meta)
				if err != nil {
					return nil, err
				}
				if !resolved {
					return nil, &BlockedNeedsGate{StepN: stepN, ToolName: toolName}
				}
				return recordGatedOutcome(log, trajectoryID, tenantID, stepN, seq+1, result)
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
		return recordGatedOutcome(log, trajectoryID, tenantID, stepN, seq+1, result)
	}
}

// recordGatedOutcome records result at the outcome slot and returns the outcome
// that stands. Two workers can finish the same claimed call: a resolver racing
// another resolver, or a resolver racing the worker still running the tool. The
// first commit wins. The loser hands back the winner's row instead of failing a
// step whose outcome is already durable.
func recordGatedOutcome(log *nodelog.NodeLog, trajectoryID, tenantID string, stepN, seq int, result any) (any, error) {
	err := appendToolResult(log, trajectoryID, tenantID, stepN, seq, result)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, nodelog.ErrSlotConflict) {
		return nil, err
	}
	if failed := storedIntegerFailure(log, trajectoryID, tenantID, stepN, seq); failed != nil {
		return nil, failed
	}
	stored, found, readErr := storedToolResult(log, trajectoryID, tenantID, stepN, seq)
	if readErr != nil {
		return nil, readErr
	}
	if !found {
		return nil, err
	}
	return stored, nil
}

// appendToolResult records {"result": result} at (stepN, seq). A result with an
// integer outside the RFC 8785 range records the integer_domain failure row
// instead and returns UnsafeInteger (spec §6.2).
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
	var unsafe *UnsafeInteger
	if !errors.As(err, &unsafe) {
		return err
	}
	if _, err := log.Append(
		"TOOL_RESULT",
		&step,
		map[string]any{
			"error":         integerDomain,
			"message":       unsafe.Message,
			"result_digest": resultDigest(result),
		},
		trajectoryID,
		tenantID,
		seq,
	); err != nil {
		return err
	}
	return unsafe
}

// resultDigest is SHA-256 over the compact sorted JSON of result, escaped the
// way Python's json.dumps does so both clients agree on strings, integers, and
// containers. Float text can differ. A value JSON cannot encode hashes its %v
// form where Python uses repr.
func resultDigest(result any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	var text string
	if err := enc.Encode(result); err != nil {
		text = fmt.Sprintf("%v", result)
	} else {
		text = asciiJSON(strings.TrimSuffix(buf.String(), "\n"))
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func asciiJSON(text string) string {
	var b strings.Builder
	for _, r := range text {
		switch {
		case r < utf8.RuneSelf:
			b.WriteRune(r)
		case r < 0x10000:
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&b, "\\u%04x\\u%04x", r1, r2)
		}
	}
	return b.String()
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
	stored, found, err := log.GetNodePayload(trajectoryID, tenantID, &stepN, seq, "TOOL_CALL")
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
	payload, found, err := log.GetNodePayload(trajectoryID, tenantID, &stepN, seq, "TOOL_RESULT")
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
	payload, found, err := log.GetNodePayload(trajectoryID, tenantID, &stepN, seq, "TOOL_RESULT")
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
