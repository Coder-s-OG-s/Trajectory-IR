package durable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
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
		return decodeStoredMemo(workflowID, stepKey, raw)
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
	stored, found, err := b.Load(ctx, workflowID, stepKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("durable: memo %s/%s missing after save", workflowID, stepKey)
	}
	return decodeStoredMemo(workflowID, stepKey, stored)
}

func decodeStoredMemo(workflowID, stepKey string, raw []byte) (any, error) {
	out, err := decodeMemo(raw)
	if err != nil {
		return nil, fmt.Errorf("durable: decode memo %s/%s: %w", workflowID, stepKey, err)
	}
	return out, nil
}

func decodeMemo(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing data")
		}
		return nil, err
	}
	return coerceMemoValue(out), nil
}

func coerceMemoValue(v any) any {
	switch n := v.(type) {
	case json.Number:
		return coerceMemoNumber(n)
	case map[string]any:
		for k, child := range n {
			n[k] = coerceMemoValue(child)
		}
		return n
	case []any:
		for i := range n {
			n[i] = coerceMemoValue(n[i])
		}
		return n
	default:
		return v
	}
}

func coerceMemoNumber(n json.Number) any {
	s := string(n)
	if memoIntegerText(s) {
		if memoIntegerOutOfRange(s) {
			return n
		}
		f, err := n.Float64()
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return n
		}
		if strconv.FormatFloat(f, 'f', 0, 64) != s {
			return n
		}
		return f
	}
	f, err := n.Float64()
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return n
	}
	return f
}

func memoIntegerOutOfRange(s string) bool {
	if s[0] == '-' {
		s = s[1:]
	}
	const maxDigits = "9007199254740991"
	if len(s) != len(maxDigits) {
		return len(s) > len(maxDigits)
	}
	return s > maxDigits
}

func memoIntegerText(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		if len(s) == 1 {
			return false
		}
		i = 1
	}
	if s[i] == '0' {
		return len(s) == i+1
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Infer is Step with a conventional step key prefix for model calls.
func Infer(ctx context.Context, b Backend, workflowID, name string, fn func(context.Context) (any, error)) (any, error) {
	return Step(ctx, b, workflowID, "infer:"+name, fn)
}

// Tool is Step with a conventional step key prefix for tool calls.
func Tool(ctx context.Context, b Backend, workflowID, name string, fn func(context.Context) (any, error)) (any, error) {
	return Step(ctx, b, workflowID, "tool:"+name, fn)
}
