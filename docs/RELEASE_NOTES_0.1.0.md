# Trajectory IR v0.1.0

First numbered library release of **Trajectory IR**: a portable, hash-verifiable intermediate representation for agent runs (seals, effect classes, block-and-gate, `.tir` packages) on top of pluggable durable backends.

## Highlights

- **Python IR core** with DBOS-backed durable steps and SQLite NodeLog
- **Optional backends**: Postgres NodeLog, Restate-style durable hooks (local memo + injectable `make_run_step`), S3 and filesystem CAS
- **Go IR stack** (`go/trajir`) with LocalSQLite/Memory memoization, optional Temporal, and filesystem CAS
- **Portable `.tir`** thin/fat packages (Python + Go), with verification, resource limits, and thin rehydrate via CAS
- **Conformance R01–R08** under `pytest conformance/`
- **Sandbox (R06)**, **graft (R07)**, **projection redaction (R08)**, **projector budget safety (R04)** plus optional projector policy file
- **Host loop example** (`examples/host_loop`) using only public client APIs
- **OSS process**: DCO, CI (Ruff/Mypy/pytest coverage floors, Package smoke, Security pip-audit, Go coverage floor + govulncheck), Dependabot, SECURITY.md

## Install (from source)

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
pip install -e ".[dev]"
# optional: pip install -e ".[s3]" or ".[postgres]"
```

From the release tag / PyPI (when published):

```bash
pip install trajectory-ir==0.1.0
```

Go:

```bash
cd go && go test ./...
```

See [QUICKSTART.md](../QUICKSTART.md) and [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md).

## Not in 0.1.0

- Package digital signatures (`SIGNATURE` reserved)
- Live Restate cluster product packaging (adapter + local memo only)
- Fluid / k8s-fluid profile
- Full multi-tenant SaaS control plane
- Automated PyPI Trusted Publishing (manual upload optional)

## Spec note

Master README still names DBOS as the Phase 1A Python default; Go production Temporal is implemented. Tracking residual wording in issue #67.

## CNCF context

Positioned against the storage gap described in the CNCF TAG Infrastructure white paper:

https://www.cncf.io/wp-content/uploads/2026/07/DoK_DataAnalyticsAIMLWorkloads_070826.pdf

## Full changelog

See [CHANGELOG.md](../CHANGELOG.md) section `[0.1.0]`.
