# Phase 1A status (as of v0.1.0)

Honest inventory of what is **on `main`** versus what remains **out of scope** or deferred. Normative detail stays in the root `README.md`.

## Shipped (code + runnable checks)

| Area | Location / notes |
|------|------------------|
| Python IR core | `pkg/trajectory_ir/` — nodes, NodeLog, effects, gate, run_step |
| DBOS adapter | `drivers/durable_backend/dbos/` |
| Restate adapter (local memo) | `drivers/durable_backend/restate/` + injectable `make_run_step` hooks |
| Python client | `client/python/trajectory_client.py` (open/project/seal/exec/commit/resume; sandbox mode) |
| `.tir` thin/fat | `pkg/trajectory_ir/package/`, Go `go/trajir/tir` |
| FS CAS + thin rehydrate | `trajectory_ir.storage`, `put_artifact` |
| S3 CAS | `drivers.s3.S3CAS` |
| Postgres NodeLog | `drivers.postgres.PostgresNodeLog` |
| Projector (R04) + policy file | `runtime/projector.py`, `runtime/policy.py`, Go `trajir/projector` |
| Sandbox (R06) | `runtime/sandbox.py`, Go `trajir/sandbox` |
| Graft (R07) | `runtime/graft.py`, Go `trajir/graft` |
| Redaction (R08 + export) | `runtime/redact.py`, Go `trajir/redact` |
| Go IR stack | `go/trajir/*` — nodes, log, effects, durable, Temporal, resume, client, cas |
| Demos / host example | `examples/kill-mid-deploy/`, `examples/host_loop/`, Go kill-mid-deploy |
| CI | Phase A: DCO, Quality, Package smoke, Security (pip-audit), Go; Phase B: Integration (Postgres), Integration (MinIO) |

### Conformance (README §10)

| ID | Runnable |
|----|----------|
| R01 | `conformance/r01_seal_resume_test.py` |
| R02 | `conformance/r02_non_idempotent_test.py` |
| R03 | `conformance/r03_pure_recompute_test.py` |
| R04 | `conformance/r04_constraint_budget_test.py` |
| R05 | `conformance/r05_tir_roundtrip_test.py` |
| R06 | `conformance/r06_sandbox_test.py` |
| R07 | `conformance/r07_graft_test.py` |
| R08 | `conformance/r08_projection_redaction_test.py` |

Phase 1A **completion bar** in the master README still treats R01/R02 as the hard product gate; R03–R08 are implemented and stay green in CI.

## Explicitly not shipped (do not claim)

| Item | Notes |
|------|--------|
| Package digital signatures | `SIGNATURE` reserved / null |
| Live Restate cluster product packaging | Adapter + local memo only; operator wires real cluster |
| Fluid / k8s-fluid profile | Later phase |
| Full projector-policy expression DSL | File/YAML subset for defaults shipped; no plugin language |
| Multi-tenant SaaS control plane | Library trust model |
| Automated PyPI Trusted Publishing | Manual tag + optional twine; see `docs/RELEASE.md` |

## Maintainer checklist (after large feature landings)

1. `main` green on latest CI run.
2. Branch protection requires: `DCO`, `Quality (Python 3.11)`, `Quality (Python 3.12)`, `Package smoke`, `Security (pip-audit)`, `Go` — see [maintainer-branch-protection.md](maintainer-branch-protection.md). Prefer **require branches to be up to date**.
3. Prefer org **secret scanning** + **push protection** enabled.
4. No open issues for work that already merged (close with PR reference).
5. Quickstart and this status doc match reality (no fictional APIs).

## CNCF context (external)

- White paper PDF: https://www.cncf.io/wp-content/uploads/2026/07/DoK_DataAnalyticsAIMLWorkloads_070826.pdf  
- Announcement: https://www.cncf.io/report-whitepaper/2026/07/08/the-cncf-data-storage-in-cloud-native-ai-white-paper/
