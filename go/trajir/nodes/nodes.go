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
	"encoding"
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

// UnsafeIntegerError reports an integer outside the RFC 8785 range. The value
// cannot be hashed or stored as a number.
type UnsafeIntegerError struct {
	Message string
}

func (e *UnsafeIntegerError) Error() string {
	if e == nil || e.Message == "" {
		return defaultUnsafeIntegerMessage
	}
	return e.Message
}

const defaultUnsafeIntegerMessage = "integer exceeds safe integer domain for JSON floats"

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
			return &UnsafeIntegerError{Message: fmt.Sprintf("%d exceeds safe integer domain for JSON floats", u)}
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
	case reflect.Struct:
		if _, ok := v.(json.Marshaler); ok {
			return nil
		}
		if _, ok := v.(encoding.TextMarshaler); ok {
			return nil
		}
		for _, f := range jsonStructFields(rv) {
			if f.asString && isJSONStringTagValue(f.val) {
				continue
			}
			if err := rejectUnsafeValues(f.val.Interface()); err != nil {
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
		return &UnsafeIntegerError{Message: fmt.Sprintf("%d exceeds safe integer domain for JSON floats", n)}
	}
	return nil
}

// IsUnsafeIntegerLiteral reports whether s is a JSON integer literal (no decimal
// point or exponent) outside the RFC 8785 range -(2^53-1)..2^53-1.
func IsUnsafeIntegerLiteral(s string) bool {
	return jsonIntegerLiteral(s) && jsonIntegerOutOfRange(s)
}

// MarshalJSON encodes v to JSON while preserving explicit float syntax (".0") on
// finite floats whose magnitude exceeds 2^53-1 when Go's encoder would otherwise
// emit an undecorated integer token (e.g. float64(1e18) -> 1000000000000000000.0).
// Decoders using json.Decoder.UseNumber() or Python json.loads then reload them
// as floats instead of out-of-range integer literals.
func MarshalJSON(v any) ([]byte, error) {
	rewritten, _ := preserveLargeFloats(v)
	return json.Marshal(rewritten)
}

func checkJSONNumber(n json.Number) error {
	s := string(n)
	if IsUnsafeIntegerLiteral(s) {
		return &UnsafeIntegerError{Message: fmt.Sprintf("%s exceeds safe integer domain for JSON floats", s)}
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

var anyReflectType = reflect.TypeOf((*any)(nil)).Elem()

func preserveLargeFloats(v any) (any, bool) {
	if v == nil {
		return nil, false
	}
	switch x := v.(type) {
	case string, bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number, json.RawMessage:
		return v, false
	case float32, float64:
		if num, ok := largeFloatNumber(x); ok {
			return num, true
		}
		return v, false
	case map[string]any:
		var copied map[string]any
		for k, child := range x {
			next, changed := preserveLargeFloats(child)
			if !changed {
				continue
			}
			if copied == nil {
				copied = make(map[string]any, len(x))
				for ck, cv := range x {
					copied[ck] = cv
				}
			}
			copied[k] = next
		}
		if copied != nil {
			return copied, true
		}
		return v, false
	case []any:
		var copied []any
		for i, child := range x {
			next, changed := preserveLargeFloats(child)
			if !changed {
				continue
			}
			if copied == nil {
				copied = make([]any, len(x))
				copy(copied, x)
			}
			copied[i] = next
		}
		if copied != nil {
			return copied, true
		}
		return v, false
	}

	if _, ok := v.(json.Marshaler); ok {
		return v, false
	}
	if _, ok := v.(encoding.TextMarshaler); ok {
		return v, false
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return v, false
		}
		next, changed := preserveLargeFloats(rv.Elem().Interface())
		if !changed {
			return v, false
		}
		return next, true
	case reflect.Float32, reflect.Float64:
		if num, ok := largeFloatNumber(v); ok {
			return num, true
		}
		return v, false
	case reflect.Slice, reflect.Array:
		return preserveLargeFloatSlice(rv)
	case reflect.Map:
		return preserveLargeFloatMap(rv)
	case reflect.Struct:
		return preserveLargeFloatStruct(rv)
	default:
		return v, false
	}
}

func preserveLargeFloatSlice(rv reflect.Value) (any, bool) {
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return rv.Interface(), false
	}
	n := rv.Len()
	var copied []any
	for i := 0; i < n; i++ {
		next, changed := preserveLargeFloats(rv.Index(i).Interface())
		if !changed {
			continue
		}
		if copied == nil {
			copied = make([]any, n)
			for j := 0; j < n; j++ {
				copied[j] = rv.Index(j).Interface()
			}
		}
		copied[i] = next
	}
	if copied != nil {
		return copied, true
	}
	return rv.Interface(), false
}

func preserveLargeFloatMap(rv reflect.Value) (any, bool) {
	if rv.IsNil() {
		return rv.Interface(), false
	}
	keys := rv.MapKeys()
	var copied reflect.Value
	for i, k := range keys {
		rawVal := rv.MapIndex(k).Interface()
		next, changed := preserveLargeFloats(rawVal)
		if !changed {
			if copied.IsValid() {
				setReflectAnyMap(copied, k, rawVal)
			}
			continue
		}
		if !copied.IsValid() {
			mapType := reflect.MapOf(rv.Type().Key(), anyReflectType)
			copied = reflect.MakeMapWithSize(mapType, len(keys))
			for j := 0; j < i; j++ {
				setReflectAnyMap(copied, keys[j], rv.MapIndex(keys[j]).Interface())
			}
		}
		setReflectAnyMap(copied, k, next)
	}
	if copied.IsValid() {
		return copied.Interface(), true
	}
	return rv.Interface(), false
}

func setReflectAnyMap(m, k reflect.Value, val any) {
	if val == nil {
		m.SetMapIndex(k, reflect.Zero(anyReflectType))
		return
	}
	m.SetMapIndex(k, reflect.ValueOf(val))
}

func largeFloatNumber(v any) (json.Number, bool) {
	f := reflect.ValueOf(v).Float()
	if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) <= float64(maxSafeInteger) {
		return "", false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	s := string(b)
	if strings.ContainsAny(s, ".eE") {
		return "", false
	}
	return json.Number(s + ".0"), true
}

func preserveLargeFloatStruct(rv reflect.Value) (any, bool) {
	var overrides map[string]any
	for _, f := range jsonStructFields(rv) {
		if f.asString && isJSONStringTagValue(f.val) {
			continue
		}
		next, changed := preserveLargeFloats(f.val.Interface())
		if !changed {
			continue
		}
		if overrides == nil {
			overrides = make(map[string]any)
		}
		overrides[f.name] = next
	}
	if overrides == nil {
		return rv.Interface(), false
	}
	raw, err := json.Marshal(rv.Interface())
	if err != nil {
		return rv.Interface(), false
	}
	decoded, err := decodeJSON(raw)
	if err != nil {
		return rv.Interface(), false
	}
	obj, ok := decoded.(map[string]any)
	if !ok {
		return rv.Interface(), false
	}
	for name, next := range overrides {
		if cur, exists := obj[name]; exists {
			if _, isStr := cur.(string); !isStr {
				obj[name] = next
			}
		}
	}
	return obj, true
}

type jsonStructField struct {
	name     string
	val      reflect.Value
	depth    int
	tagged   bool
	asString bool
}

func jsonStructFields(rv reflect.Value) []jsonStructField {
	type queueItem struct {
		val   reflect.Value
		depth int
	}
	queue := []queueItem{{val: rv, depth: 0}}
	var candidates []jsonStructField
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		rt := cur.val.Type()
		for i := 0; i < rt.NumField(); i++ {
			sf := rt.Field(i)
			name, tagged, skip, asString := parseJSONFieldTag(sf)
			if skip {
				continue
			}
			ft := sf.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if !sf.IsExported() && (!sf.Anonymous || ft.Kind() != reflect.Struct) {
				continue
			}
			fv := cur.val.Field(i)
			if sf.Anonymous && name == "" && ft.Kind() == reflect.Struct {
				for fv.Kind() == reflect.Ptr && !fv.IsNil() {
					fv = fv.Elem()
				}
				if fv.Kind() == reflect.Struct {
					queue = append(queue, queueItem{val: fv, depth: cur.depth + 1})
				}
				continue
			}
			if name == "" {
				name = sf.Name
			}
			candidates = append(candidates, jsonStructField{
				name:     name,
				val:      fv,
				depth:    cur.depth,
				tagged:   tagged,
				asString: asString,
			})
		}
	}
	return dominantStructFields(candidates)
}

func dominantStructFields(candidates []jsonStructField) []jsonStructField {
	type rank struct {
		depth  int
		tagged bool
		count  int
	}
	best := make(map[string]rank, len(candidates))
	for _, c := range candidates {
		r, ok := best[c.name]
		if !ok || c.depth < r.depth || (c.depth == r.depth && c.tagged && !r.tagged) {
			best[c.name] = rank{depth: c.depth, tagged: c.tagged, count: 1}
		} else if c.depth == r.depth && c.tagged == r.tagged {
			r.count++
			best[c.name] = r
		}
	}
	out := make([]jsonStructField, 0, len(best))
	for _, c := range candidates {
		r := best[c.name]
		if r.count == 1 && c.depth == r.depth && c.tagged == r.tagged {
			out = append(out, c)
		}
	}
	return out
}

func parseJSONFieldTag(sf reflect.StructField) (name string, tagged, skip, asString bool) {
	tag, ok := sf.Tag.Lookup("json")
	if !ok {
		return "", false, false, false
	}
	if tag == "-" {
		return "", false, true, false
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	tagged = name != ""
	for _, opt := range parts[1:] {
		if opt == "string" {
			asString = true
		}
	}
	return name, tagged, false, asString
}

func isJSONStringTagValue(v reflect.Value) bool {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
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
