# Milestone v1.0.1 - First Buildable Core: Design

**Source:** [Coder-s-OG-s/Trajectory-IR#2](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/2)

This spec captures the repo-specific decisions for implementing Milestone v1.0.1.
The technical architecture itself is normative and already fully specified by the
issue (an execution runbook, not an open design question) — this doc exists to
record how *we* execute it: workflow, commit strategy, and the one open risk that
needs a research spike before it can be locked down.

## Definition of Done

1. `pip install -e .` works from a clean clone.
2. `pytest conformance/r01_seal_resume_test.py conformance/r02_non_idempotent_test.py` passes.
3. `python examples/kill-mid-deploy/run_demo.py` + real `kill -9` + restart → exactly
   one `deploy_server` execution, zero extra model calls.
4. Everything committed with DCO sign-off, CI green.

## Architecture

A durable execution core built on DBOS. Every agent step (project context → model
call → decision seal → tool execution → commit) runs as a durable workflow. The
governing invariant: nothing with cost or side effects (model inference, tool
calls) may re-run silently across a crash-resume.

**Critical fix carried from spec review (issue §1):** the master spec wraps
`DECISION`/`TOOL_CALL` as durable steps but is ambiguous about model inference
itself. Without wrapping, DBOS replays the whole workflow body on resume and the
model gets re-invoked (wastefully at minimum, and it falsifies R01's literal claim
that "the model is not invoked again"). Fix: `durable_infer()` wraps model calls
identically to `durable_tool()`. This is enforced in `resume/run_step()`, not just
documented.

## Components

| Path | Responsibility |
|---|---|
| `pkg/trajectory_ir/runtime/nodes.py` | `Node` dataclass, `payload_hash()` (RFC 8785 JCS → SHA-256, `ts` excluded from hash), `node_id()`. Foundation everything else depends on. |
| `drivers/durable-backend/dbos/adapter.py` | `init_backend()`, `durable_infer()`, `durable_tool()`, `durable_workflow()`. Convention: model/tool calls are never invoked raw inside a workflow. |
| `pkg/trajectory_ir/effects/` | `EffectClass` enum + `classify_from_mcp()`. Fail-closed: any missing/ambiguous MCP annotation → `NON_IDEMPOTENT_WRITE`. |
| `pkg/trajectory_ir/resume/` | `run_step()` workflow + `block_and_gate()`. Where the §1 fix is enforced in code. |
| `client/python/` | Exactly 6 calls: `open_trajectory`, `project`, `seal_decision`, `exec_tool`, `commit_step`, `resume`. No 7th call this milestone. |
| `conformance/` | R01/R02 crash-injection tests — real subprocess + `SIGKILL`, not in-process, since durability across process death is the actual claim under test. |
| `examples/kill-mid-deploy/` | Narratable version of R02, recorded via asciinema — the launch asset. |

## Data Flow

`project()` builds context → `durable_infer(model_call)` produces a plan →
`DECISION` node sealed → tools dispatched via `durable_tool`, with
`NON_IDEMPOTENT_WRITE` tools routed through `block_and_gate` → `COMMIT_STEP`. On
crash, DBOS replays the workflow but skips already-completed (memoized) steps —
including inference — so a crash after seal never re-invokes the model. A crash
mid non-idempotent tool call lands as `BLOCKED_NEEDS_GATE` rather than
auto-retrying.

## Open Risk — Research Spike Required

`block_and_gate()`'s detection of "workflow was in progress, no result recorded"
depends on DBOS's exact workflow-status query API. The issue itself flags this as
unconfirmed and tells us to check current docs
(`docs.dbos.dev/python/tutorials/workflow-tutorial`) rather than guess, since
R01/R02 depend on it. This will be resolved as part of the seal/resume
implementation step, not assumed up front.

## Testing Strategy

- **Unit tests** (fast, in-process): node identity/hashing (same payload,
  different key order → same hash; `ts` never affects hash), effect-class
  fail-closed behavior.
- **Conformance tests** (slow, subprocess + `SIGKILL`): R01 (no re-inference
  after seal, asserted via a call counter, not just "tools ran once"), R02
  (non-idempotent tool crash → `BLOCKED_NEEDS_GATE`, no auto-retry). Expect
  timing flakiness in the sentinel-file approach — budget for it, it's normal for
  this test shape.
- **Demo**: `examples/kill-mid-deploy/run_demo.py`, manual `kill -9`, `--resume`,
  recorded once reliable.

## Workflow & Commit Strategy

- Follow the issue's own §12 build order: scaffold → node model → DBOS adapter →
  effect classes → seal/resume → client SDK → R01/R02 conformance → demo.
- TDD rhythm (test red → commit → implement green → commit) applies to sections
  with real logic to verify: node hashing, effect classification, seal/resume,
  and the R01/R02 conformance tests. Structural sections without meaningful
  test-first steps — env setup, repo scaffold, the adapter's thin DBOS wrappers,
  the client SDK's thin call wrappers — get one commit each instead of a forced
  pair.
- Conventional prefixes matching the existing git log style: `feat:`, `test:`,
  `chore:`, `docs:`, `build:`.
- A formal implementation plan (via the writing-plans skill) follows this spec,
  broken into the same section order, before any code is written.

## Out of Scope (this milestone)

- Postgres backend (SQLite is the default and sufficient for local dev per the
  issue; Postgres is a production concern, not this milestone's).
- Any client SDK call beyond the 6 named.
- `drivers/sqlite`, `drivers/postgres`, `drivers/s3` beyond scaffolding the
  directories — the durable-backend/dbos driver is the only one built out this
  milestone.
