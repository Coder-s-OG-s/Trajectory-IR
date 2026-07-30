package durable

import (
	"context"
	"encoding/json"
	"fmt"
)

// Backend stores memoized step outputs for a workflow id.
// Temporal and Restate drivers will implement this later; LocalSQLite and
// Memory implement it for Phase 1B coding and tests.
type Backend interface {
	// Load returns the saved JSON for workflowID+stepKey if present.
	Load(ctx context.Context, workflowID, stepKey string) (value []byte, found bool, err error)
	// Save stores JSON for workflowID+stepKey. Saving twice with the same key
	// should be idempotent (keep first value).
	Save(ctx context.Context, workflowID, stepKey string, value []byte) error
	// Close releases resources. Safe to call more than once.
	Close() error
}

// Step runs fn once per (workflowID, stepKey). On later calls it returns the
// memoized JSON decoded into result, without calling fn.
//
// result must be a pointer if non-nil when loading a prior value. When fn runs,
// its return value is JSON marshaled. Use map[string]any or concrete structs
// that encode cleanly.
func Step(ctx context.Context, b Backend, workflowID, stepKey string, fn func(context.Context) (any, error)) (any, error) {
	if b == nil {
		return nil, fmt.Errorf("durable: nil backend")
	}
	if workflowID == "" || stepKey == "" {
		return nil, fmt.Errorf("durable: workflowID and stepKey are required")
	}

	raw, found, err := b.Load(ctx, workflowID, stepKey)
	if err != nil {
		return nil, err
	}
	if found {
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("durable: decode memo %s/%s: %w", workflowID, stepKey, err)
		}
		return out, nil
	}

	val, err := fn(ctx)
	if err != nil {
		return nil, err
	}
	raw, err = json.Marshal(val)
	if err != nil {
		return nil, fmt.Errorf("durable: encode memo %s/%s: %w", workflowID, stepKey, err)
	}
	if err := b.Save(ctx, workflowID, stepKey, raw); err != nil {
		return nil, err
	}
	// Return the same JSON round trip shape callers get on replay.
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Infer is Step with a conventional step key prefix for model calls.
func Infer(ctx context.Context, b Backend, workflowID, name string, fn func(context.Context) (any, error)) (any, error) {
	return Step(ctx, b, workflowID, "infer:"+name, fn)
}

// Tool is Step with a conventional step key prefix for tool calls.
func Tool(ctx context.Context, b Backend, workflowID, name string, fn func(context.Context) (any, error)) (any, error) {
	return Step(ctx, b, workflowID, "tool:"+name, fn)
}
