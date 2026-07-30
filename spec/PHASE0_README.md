# Phase 0 — Trajectory IR spec freeze

**Goal:** Schema + seal format first. Repo structure and code only after **`spec-v0.1`** tag.

## Five freeze docs

| Doc | Contents |
|-----|----------|
| [node-kinds.md](./node-kinds.md) | v0.1 nodes + `node_id` hash + **linear known-args rule** |
| [effect-classes.md](./effect-classes.md) | 6 classes + MCP mapping (silent → NON_IDEMPOTENT) |
| [seals-and-resume.md](./seals-and-resume.md) | Resume matrix + **block-and-gate** |
| [traj-package.md](./traj-package.md) | **`.tir`** package (not `.traj`) fat/thin/redacted |
| [conformance.md](./conformance.md) | R01–R08 runnable tests |

## Critical linear rule (v0.1)

Sealed tool list valid **only if all args known at seal time**.  
If B needs A’s result → **B is next step / next inference**.  
This is why linear works without plan DAG.

## Package extension

**`.tir`** — avoids collision with SWE-agent’s `.traj`.

## Suggested work split (from review)

| Owner | Work |
|-------|------|
| Design | These 5 docs (review + edit) |
| Runtime | Conformance harness + skeleton after tag |
| Package/demo | `.tir` import/export + kill-mid-deploy script after tag |

## Timeline

1. **All three review** these docs  
2. Tag **`spec-v0.1`**  
3. **Then** Phase 1A: SQLite IR log + Python SDK + R01/R02 green + kill-resume demo  

**One project:** Trajectory IR core. CLOOP store / CAMI K8s = later layers, not parallel frameworks.
