// Package nodes implements Trajectory IR node identity for the Go IR core.
//
// Hashing matches the Python reference in pkg/trajectory_ir/runtime/nodes.py:
// RFC 8785 JCS of the payload, then SHA-256 hex for payload_hash; node_id is
// SHA-256 hex of tenant|trajectory|step|seq|kind|payload_hash with the same
// string rules as Python (step None formats as the four characters None).
package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// NodeID builds the content-addressed id string used by the Python reference.
// stepN nil is formatted as "None" so it matches Python f-string None.
func NodeID(tenantID, trajectoryID string, stepN *int, seq int, kind, phash string) (string, error) {
	if err := ValidateIDComponent("tenant_id", tenantID); err != nil {
		return "", err
	}
	if err := ValidateIDComponent("trajectory_id", trajectoryID); err != nil {
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
