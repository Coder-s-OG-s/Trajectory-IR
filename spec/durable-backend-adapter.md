# Durable backend adapter interface

| | |
|---|---|
| **Status** | Placeholder for Phase 1A |
| **Owner** | Lands with the first DBOS adapter implementation |
| **Normative parent** | Root `README.md` §3.1, §12.0, §13 |

## Intent

Every durable-execution driver under `drivers/durable-backend/` must satisfy
one shared adapter contract so that:

- `DECISION` and `TOOL_CALL` (and model inference for R01) run as backend steps
- crash/replay/lease behavior comes from the backend, not from custom code in
  `pkg/trajectory_ir/resume/`
- swapping DBOS for Restate later does not change the public SDK

## What this file is not (yet)

This document does **not** invent method names, retry policies, or status
enums ahead of the first adapter. Those will be written in the same PR that
implements `drivers/durable-backend/dbos/`, and must match the master README.

Until then: do not add parallel crash-detection or retry loops in `pkg/`.
