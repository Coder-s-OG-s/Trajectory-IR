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
	if len(value) > 512 {
		return fmt.Errorf("%s exceeds maximum length 512", name)
	}
	return nil
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
	if err := rejectUnsafeIntegers(payload); err != nil {
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
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("invalid character after top-level value")
		}
		return nil, err
	}
	return obj, nil
}

func rejectUnsafeIntegers(v any) error {
	if v == nil {
		return nil
	}
	if n, ok := v.(json.Number); ok {
		return checkJSONNumber(n)
	}
	if _, ok := v.(time.Time); ok {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Interface, reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return rejectUnsafeIntegers(rv.Elem().Interface())
	case reflect.Map:
		iter := rv.MapRange()
		for iter.Next() {
			if err := rejectUnsafeIntegers(iter.Value().Interface()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice {
			if raw, ok := v.(json.RawMessage); ok {
				return rejectRawMessage(raw)
			}
			if rv.Type().Elem().Kind() == reflect.Uint8 {
				return nil
			}
		}
		for i := 0; i < rv.Len(); i++ {
			if err := rejectUnsafeIntegers(rv.Index(i).Interface()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			sf := rt.Field(i)
			if !sf.IsExported() {
				continue
			}
			tag := sf.Tag.Get("json")
			if tag == "-" || strings.HasPrefix(tag, "-,") {
				continue
			}
			if err := rejectUnsafeIntegers(rv.Field(i).Interface()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return checkSigned(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return checkUnsigned(rv.Uint())
	default:
		return nil
	}
}

func rejectRawMessage(raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil {
		return nil
	}
	return rejectUnsafeIntegers(parsed)
}

func checkSigned(n int64) error {
	if n > maxSafeInteger || n < -maxSafeInteger {
		return fmt.Errorf("%d exceeds safe integer domain for JSON floats", n)
	}
	return nil
}

func checkUnsigned(n uint64) error {
	if n > uint64(maxSafeInteger) {
		return fmt.Errorf("%d exceeds safe integer domain for JSON floats", n)
	}
	return nil
}

func checkJSONNumber(n json.Number) error {
	s := string(n)
	if jsonIntegerLiteral(s) && jsonIntegerOutOfRange(s) {
		return fmt.Errorf("%s exceeds safe integer domain for JSON floats", s)
	}
	return nil
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
