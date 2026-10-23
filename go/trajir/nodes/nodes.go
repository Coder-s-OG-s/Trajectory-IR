// Package nodes implements Trajectory IR node identity for the Go IR core.
//
// Hashing matches the Python reference in pkg/trajectory_ir/runtime/nodes.py:
// RFC 8785 JCS of the payload, then SHA-256 hex for payload_hash; node_id is
// SHA-256 hex of tenant|trajectory|step|seq|kind|payload_hash with the same
// string rules as Python (step None formats as the four characters None).
package nodes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
)

// NODE_KINDS is the Phase 1A set used by the Python reference.
var NODE_KINDS = map[string]struct{}{
	"INPUT":           {},
	"CONSTRAINT":      {},
	"STATE_SET":       {},
	"PROJECT_CONTEXT": {},
	"THOUGHT":         {},
	"DECISION":        {},
	"TOOL_CALL":       {},
	"TOOL_RESULT":     {},
	"ARTIFACT_PUT":    {},
	"ARTIFACT_REF":    {},
	"LTM_QUERY":       {},
	"LTM_HIT":         {},
	"LTM_PROJECT":     {},
	"COMMIT_STEP":     {},
	"ABORT":           {},
	"REDACTION":       {},
}

// ValidateIDComponent rejects empty values and characters that break the
// '|' joined node_id preimage (delimiter / control injection).
func ValidateIDComponent(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s must be a non-empty string", name)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", name)
	}
	if strings.ContainsAny(value, "|\n\r\x00") {
		return fmt.Errorf("%s must not contain '|' or control characters (identity delimiter safety)", name)
	}
	if utf8.RuneCountInString(value) > 512 {
		return fmt.Errorf("%s exceeds maximum length 512", name)
	}
	return nil
}

// DecodePayloadJSON parses a stored payload_json column. Numbers stay as
// json.Number so integers above 2^53 survive a read and re-hash unchanged, and
// anything after the first JSON value is rejected as corruption.
func DecodePayloadJSON(raw string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("payload_json has trailing data after the JSON value")
	}
	return payload, nil
}

// PayloadHash returns SHA-256 hex of the RFC 8785 canonical form of payload.
// The payload must not contain a "ts" key (wall clock is never hashed).
func PayloadHash(payload map[string]any) (string, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["ts"]; ok {
		return "", fmt.Errorf("wall-clock ts must never be hashed (spec 6.3)")
	}
	if err := rejectUnsafeValues(payload); err != nil {
		return "", err
	}
	// encoding/json emits a legal JSON encoding; jcs.Transform applies RFC 8785.
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("json marshal: %w", err)
	}
	canon, err := jcs.Transform(raw)
	if err != nil {
		return "", fmt.Errorf("jcs: %w", err)
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

const maxSafeInteger int64 = 1<<53 - 1

// DecodeObject parses one JSON object and preserves number literals.
func DecodeObject(raw []byte) (map[string]any, error) {
	v, err := decodeJSON(raw)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("want JSON object, got %T", v)
	}
	return obj, nil
}

// AsFloat64 reads a JSON number as float64 (float, int, or json.Number).
func AsFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// AsInt reads a JSON number as int.
func AsInt(v any) (int, bool) {
	f, ok := AsFloat64(v)
	if !ok {
		return 0, false
	}
	return int(f), true
}

func decodeJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("invalid character after top-level value")
		}
		return nil, err
	}
	return v, nil
}

func rejectUnsafeValues(v any) error {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case json.Number:
		return checkJSONNumber(n)
	case json.RawMessage:
		decoded, err := decodeJSON(n)
		if err != nil {
			return err
		}
		return rejectUnsafeValues(decoded)
	case map[string]any:
		for _, child := range n {
			if err := rejectUnsafeValues(child); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for _, child := range n {
			if err := rejectUnsafeValues(child); err != nil {
				return err
			}
		}
		return nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return rejectUnsafeValues(rv.Elem().Interface())
	case reflect.Float32, reflect.Float64:
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return checkInt64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := rv.Uint()
		if u > uint64(maxSafeInteger) {
			return fmt.Errorf("%d exceeds safe integer domain for JSON floats", u)
		}
		return nil
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if err := rejectUnsafeValues(rv.Index(i).Interface()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		iter := rv.MapRange()
		for iter.Next() {
			if err := rejectUnsafeValues(iter.Value().Interface()); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func checkInt64(n int64) error {
	if n > maxSafeInteger || n < -maxSafeInteger {
		return fmt.Errorf("%d exceeds safe integer domain for JSON floats", n)
	}
	return nil
}

func checkJSONNumber(n json.Number) error {
	s := string(n)
	if !jsonIntegerLiteral(s) || !jsonIntegerOutOfRange(s) {
		return nil
	}
	if jsonIntegerExactFloat64(s) {
		return nil
	}
	return fmt.Errorf("%s exceeds safe integer domain for JSON floats", s)
}

func jsonIntegerExactFloat64(s string) bool {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return false
	}
	return strconv.FormatFloat(f, 'f', 0, 64) == s
}

func jsonIntegerLiteral(s string) bool {
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

func jsonIntegerOutOfRange(s string) bool {
	if s[0] == '-' {
		s = s[1:]
	}
	const maxDigits = "9007199254740991"
	if len(s) != len(maxDigits) {
		return len(s) > len(maxDigits)
	}
	return s > maxDigits
}

// NodeID builds the content-addressed id string used by the Python reference.
// stepN nil is formatted as "None" so it matches Python f-string None.
func NodeID(tenantID, trajectoryID string, stepN *int, seq int, kind, phash string) (string, error) {
	if err := ValidateIDComponent("tenant_id", tenantID); err != nil {
		return "", err
	}
	if err := ValidateIDComponent("trajectory_id", trajectoryID); err != nil {
		return "", err
	}
	if err := ValidateIDComponent("kind", kind); err != nil {
		return "", err
	}
	if err := ValidateIDComponent("payload_hash", phash); err != nil {
		return "", err
	}
	step := "None"
	if stepN != nil {
		step = strconv.Itoa(*stepN)
	}
	raw := fmt.Sprintf("%s|%s|%s|%d|%s|%s", tenantID, trajectoryID, step, seq, kind, phash)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:]), nil
}

// Node is a hashed IR event. Identity fields are computed on construction.
type Node struct {
	Kind         string
	TrajectoryID string
	TenantID     string
	StepN        *int
	Seq          int
	Payload      map[string]any
	TS           float64
	PHash        string
	ID           string
}

// NewNode validates kind and payload rules and fills PHash and ID.
func NewNode(kind, trajectoryID, tenantID string, stepN *int, seq int, payload map[string]any) (*Node, error) {
	if _, ok := NODE_KINDS[kind]; !ok {
		return nil, fmt.Errorf("unknown node kind: %s", kind)
	}
	if err := ValidateIDComponent("tenant_id", tenantID); err != nil {
		return nil, err
	}
	if err := ValidateIDComponent("trajectory_id", trajectoryID); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	phash, err := PayloadHash(payload)
	if err != nil {
		return nil, err
	}
	id, err := NodeID(tenantID, trajectoryID, stepN, seq, kind, phash)
	if err != nil {
		return nil, err
	}
	return &Node{
		Kind:         kind,
		TrajectoryID: trajectoryID,
		TenantID:     tenantID,
		StepN:        stepN,
		Seq:          seq,
		Payload:      payload,
		TS:           float64(time.Now().UnixNano()) / 1e9,
		PHash:        phash,
		ID:           id,
	}, nil
}
