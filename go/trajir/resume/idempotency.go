package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
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

// IdempotencyKey is spec §7.3: hex(sha256(domain || 0x00 || tenant || 0x00 ||
// trajectory || 0x00 || step || 0x00 || seq)).
//
// The colon form (trajectory:step:seq) leaked identifiers into HTTP headers
// and omitted tenant. This hash is stable for a sealed slot, includes tenant,
// and is short enough to send as Idempotency-Key. Hosts must forward it to
// the remote API. Recording it on TOOL_CALL is not exactly-once.
// Block-and-gate is at-most-one automatic attempt from this client.
func IdempotencyKey(tenantID, trajectoryID string, stepN, seq int) string {
	var b strings.Builder
	b.WriteString(IdempotencyDomain)
	b.WriteByte(0)
	b.WriteString(tenantID)
	b.WriteByte(0)
	b.WriteString(trajectoryID)
	b.WriteByte(0)
	b.WriteString(strconv.Itoa(stepN))
	b.WriteByte(0)
	b.WriteString(strconv.Itoa(seq))
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// IdempotencyKeyHeader returns a map a host can merge onto outbound HTTP.
func IdempotencyKeyHeader(key string) map[string]string {
	return map[string]string{"Idempotency-Key": key}
}
