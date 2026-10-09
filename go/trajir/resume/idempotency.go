package resume

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
)

// IdempotencyDomain is the SHA-256 domain separator for spec §7.3 keys.
const IdempotencyDomain = "trajir-idempotency-v1"

// CallMeta is context for one sealed tool slot. Tools that need to forward
// the key to a remote API receive it here, never as a mutated args map.
type CallMeta struct {
	TenantID       string
	TrajectoryID   string
	StepN          int
	Seq            int
	IdempotencyKey string
}

// NewCallMeta builds CallMeta with the hashed §7.3 key.
func NewCallMeta(tenantID, trajectoryID string, stepN, seq int) CallMeta {
	return CallMeta{
		TenantID:       tenantID,
		TrajectoryID:   trajectoryID,
		StepN:          stepN,
		Seq:            seq,
		IdempotencyKey: IdempotencyKey(tenantID, trajectoryID, stepN, seq),
	}
}

// IdempotencyKey is spec §7.3: hex(sha256(length-prefixed fields)).
// Each field is uint32be(byte length) || utf-8 bytes, in order:
// domain, tenant, trajectory, decimal step, decimal seq.
//
// Length prefixes stop a NUL inside a tenant or trajectory id from aliasing
// a different pair. The earlier NUL-joined draft (domain || 0x00 || tenant ||
// 0x00 || ...) collides: ("a", "b\x00c") and ("a\x00b", "c") hash the same.
//
// Resume: a trajectory whose TOOL_CALL rows were sealed with the withdrawn
// colon key (trajectory:step:seq, no tenant) does not compute this hash.
// Pre-1.0, there is no translator. Resuming that call and forwarding the new
// key can repeat the side effect at the callee. The colon form leaked
// identifiers into HTTP headers and omitted tenant.
//
// The hash is stable for a sealed slot and is short enough to send as
// Idempotency-Key. Hosts must forward it to the remote API. Recording it
// on TOOL_CALL is not exactly-once. Block-and-gate is at-most-one automatic
// attempt from this client.
func IdempotencyKey(tenantID, trajectoryID string, stepN, seq int) string {
	var buf []byte
	buf = appendLenPrefixed(buf, IdempotencyDomain)
	buf = appendLenPrefixed(buf, tenantID)
	buf = appendLenPrefixed(buf, trajectoryID)
	buf = appendLenPrefixed(buf, strconv.Itoa(stepN))
	buf = appendLenPrefixed(buf, strconv.Itoa(seq))
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

func appendLenPrefixed(dst []byte, s string) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	dst = append(dst, n[:]...)
	return append(dst, s...)
}

// IdempotencyKeyHeader returns a map a host can merge onto outbound HTTP.
func IdempotencyKeyHeader(key string) map[string]string {
	return map[string]string{"Idempotency-Key": key}
}
