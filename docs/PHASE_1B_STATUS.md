# Phase 1B status (Go primary SDK)

Honest inventory for the **current development focus** after Phase 1A / `v0.1.0`.
Normative detail lives in the root `README.md` (spec v1.3 language priority).

## Decision

| Surface | Phase 1A | Phase 1B |
|---------|----------|----------|
| Primary language | Python | **Go** |
| Python role | Primary | Reference / parity |
| Go durable production | Temporal (shipped) | Temporal (unchanged) |
| New features | Often Python first | **Go first** (or dual PR) |

Epic: [#113](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/113)

## Workstreams

| Issue | Topic | Status |
|-------|--------|--------|
| #114 | Spec: declare Go primary in README | This status doc lands with the spec PR |
| #115 | Go adoption host demo | Open |
| #116 | Go first QUICKSTART | Open |
| #117 | Go Postgres NodeLog | Open |
| #118 | Go S3 compatible CAS | Open |
| #119 | CONTRIBUTING Go first process | Open |

## Already on main (Go)

- IR: nodes, log (SQLite), effects, resume, sandbox, graft, redact, projector
- Client SDK and kill mid deploy demo
- Filesystem CAS and `.tir` thin/fat
- Temporal production durable backend; LocalSQLite/Memory test fakes
- Spec: Temporal recognized for Go (#67 / #112)

## Explicitly not Phase 1B goals

- Deleting Python or dropping R01–R08 Python CI
- Package signatures, Fluid productization, multi tenant SaaS
- Rewriting DBOS out of the Python port

## Related

- Phase 1A inventory: [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md)
- Cut patch release: issue #111 (`v0.1.1`) is independent of this epic
