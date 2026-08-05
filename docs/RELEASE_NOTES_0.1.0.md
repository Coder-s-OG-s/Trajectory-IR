# Trajectory IR v0.1.0

First numbered library release of **Trajectory IR**: a portable, hash-verifiable intermediate representation for agent runs (seals, effect classes, block-and-gate, `.tir` packages) on top of pluggable durable backends.

## Highlights

- **Python IR core** with DBOS-backed durable steps and SQLite NodeLog
- **Go IR stack** (`go/trajir`) with LocalSQLite/Memory memoization and optional Temporal
- **Portable `.tir`** thin/fat packages (Python + Go), with verification and resource limits
- **Conformance R01–R08** runnable under `pytest conformance/` (and Go package tests where noted)
- **Sandbox mode (R06)**, **graft (R07)**, **projection redaction (R08)**, **projector budget safety (R04)**
- OSS process: DCO, CI (Ruff/Mypy/pytest/govulncheck/pip-audit gate), Dependabot, SECURITY.md

## Install (from source)

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
pip install -e ".[dev]"
```

Go:

```bash
cd go && go test ./...
```

See [QUICKSTART.md](../QUICKSTART.md) and [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md).

## Not in 0.1.0

- Package digital signatures  
- Restate adapter  
- Full FS/S3 CAS product and Fluid  
- Multi-tenant SaaS control plane  
- Automated PyPI publish (optional maintainer follow-up)

## CNCF context

Positioned against the storage gap described in the CNCF TAG Infrastructure white paper:

https://www.cncf.io/wp-content/uploads/2026/07/DoK_DataAnalyticsAIMLWorkloads_070826.pdf

## Full changelog

See [CHANGELOG.md](../CHANGELOG.md) section `[0.1.0]`.
