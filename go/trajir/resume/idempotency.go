package resume

import "fmt"

// IdempotencyKey is spec §7.3: trajectory_id:step_n:stable_call_id.
//
// stable_call_id is the sealed tool slot seq (typically 2+2*i), never a
// freshly minted id after a crash. Hosts must forward this string to the
// remote API (Idempotency-Key, Stripe, cloud APIs). Recording it on
// TOOL_CALL is not exactly-once. Block-and-gate is at-most-one automatic
// attempt from this client.
func IdempotencyKey(trajectoryID string, stepN, seq int) string {
	return fmt.Sprintf("%s:%d:%d", trajectoryID, stepN, seq)
}
