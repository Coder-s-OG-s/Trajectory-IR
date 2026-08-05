# Phase 1A status (as of 0.1.0 prep)

Honest inventory of what is **on `main`** versus what remains **out of scope** or deferred. Normative detail stays in the root `README.md`.

## Shipped (code + runnable checks)

| Area | Location / notes |
|------|------------------|
| Python IR core | `pkg/trajectory_ir/` — nodes, NodeLog, effects, gate, run_step |
| DBOS adapter | `drivers/durable_backend/dbos/` |
| Python client | `client/python/trajectory_client.py` (open/project/seal/exec/commit/resume; sandbox mode) |
| `.tir` thin/fat | `pkg/trajectory_ir/package/`, Go `go/trajir/tir` |
| Projector (R04) | `runtime/projector.py`, Go `trajir/projector` (RFC 8785 size metric) |
| Sandbox (R06) | `runtime/sandbox.py`, Go `trajir/sandbox` |
| Graft (R07) | `runtime/graft.py`, Go `trajir/graft` |
| Redaction (R08 + export) | `runtime/redact.py`, Go `trajir/redact` |
| Go IR stack | `go/trajir/*` — nodes, log, effects, durable, Temporal, resume, client |
| Demos | `examples/kill-mid-deploy/`, `go/examples/kill-mid-deploy/` |
| CI | DCO, Quality (3.11/3.12: ruff, mypy, unit/e2e/conformance, pip-audit test), Go (tests + govulncheck) |

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

Phase 1A **completion bar** in the master README still treats R01/R02 as the hard product gate; R03–R08 are now implemented and should stay green in CI.

## Explicitly not shipped (do not claim)

| Item | Notes |
|------|--------|
| Package digital signatures | `SIGNATURE` reserved / null |
| Restate adapter | Spec: after DBOS is solid |
| Local FS CAS object store product | Fat `.tir` embeds bytes; thin URI rehydrate not a full CAS service |
| Postgres / S3 drivers | Package scaffolds only |
| Fluid / k8s-fluid profile | Later phase |
| Full `projector-policy.yaml` DSL | Default built-in policy only |
| Multi-tenant SaaS control plane | Library trust model |
| PyPI publish automation | Prep for 0.1.0 tag; publish is a separate maintainer action |

## Maintainer checklist (after large feature landings)

1. `main` green on latest CI run.
2. Branch protection requires: `DCO`, `Quality (Python 3.11)`, `Quality (Python 3.12)`, `Go` — see [maintainer-branch-protection.md](maintainer-branch-protection.md).
3. Prefer org **secret scanning** + **push protection** enabled.
4. No open issues for work that already merged (close with PR reference).
5. Quickstart and this status doc match reality (no fictional APIs).

## CNCF context (external)

- White paper PDF: https://www.cncf.io/wp-content/uploads/2026/07/DoK_DataAnalyticsAIMLWorkloads_070826.pdf  
- Announcement: https://www.cncf.io/report-whitepaper/2026/07/08/the-cncf-data-storage-in-cloud-native-ai-white-paper/
