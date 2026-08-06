// Package durable is the Go pluggable execution backend for Trajectory IR.
//
// Decision (issue #16)
//
//	Coding default: LocalSQLite (and Memory for pure unit tests). Both are
//	test fakes: a hand rolled first writer wins memoization store, not an
//	integration with a real durable execution engine. They memoize step
//	results by workflow id and step key so re-entry skips re-running model
//	inference and tools, which is enough for Phase 1B Go development and
//	the current conformance suite, but neither should be treated as a
//	production backend. See issue #92 for the open question on wiring
//	trajir/durable into a real DBOS or Restate client, tracked alongside #67.
//
//	Production target: Temporal as the first full Go adapter
//	(go/trajir/durable/temporal, issue #24). Restate remains a welcome
//	second adapter later, consistent with the master README.
//
// Rules (same as Python + DBOS)
//
//  1. Model inference and tool calls must run through Step (or a backend
//     wrapper that provides the same memoization). Never call them raw inside
//     a workflow body if crash resume must not re-invoke them.
//  2. Block and gate for NON_IDEMPOTENT_WRITE still uses the NodeLog
//     (TOOL_CALL at step+seq) as source of truth, not backend internals alone.
//  3. Do not implement custom lease, heartbeat, or retry loops outside this
//     package and a real Temporal/Restate driver.
package durable
