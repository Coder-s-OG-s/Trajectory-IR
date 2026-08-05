# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Agent host loop example (`examples/host_loop`) using only the public client
  SDK with a stub model and optional sandbox mode (issue #65).
- `put_artifact` helper and optional `cas=` on thin `export_tir` / `import_tir` for fail closed rehydrate (issue #73).

- Local filesystem content addressed store (`trajectory_ir.storage.FileSystemCAS`)
  with sharded `cas/<2-hex>/<hash>` layout, hash verify on get, and
  `rehydrate_artifacts` for thin `.tir` packages (issue #62).
- S3 compatible CAS driver (`drivers.s3.S3CAS`) with the same sharded key layout
  and injectable client for tests; optional `boto3` extra (issue #63).

### Changed

### Fixed

## [0.1.0] - 2026-08-05

First library-tagged release of the Phase 1A surface: dual-language IR, portable `.tir`, and runnable R01–R08.

### Added

- `put_artifact` helper and optional `cas=` on thin `export_tir` / `import_tir` for fail closed rehydrate (issue #73).

- Phase 1A buildable core (nodes, NodeLog, effects, DBOS adapter, seal/resume gate, client SDK, kill-mid-deploy, R01/R02).
- Issue templates, PR template, DCO CI job, Ruff/Mypy in CI alongside unit/e2e/conformance.
- Maintainer note for branch protection on `main`.
- Go IR core hashing package under `go/` with shared `testdata/hash_vectors.json` parity tests against Python.
- Go SQLite NodeLog under `go/trajir/log` matching the Python append only IR log.
- Go effect classes and fail closed MCP mapping under `go/trajir/effects`.
- Go durable backend package (`trajir/durable`): local SQLite and memory step memoization; Temporal named as production target.
- Go block and gate under `go/trajir/resume` for non idempotent tool re-entry.
- Go RunStep seal path (project, durable infer, DECISION, tools, COMMIT_STEP).
- Go crash resume conformance tests (R01/R02 style) via cmd/crashagent.
- Go Temporal durable backend adapter under `go/trajir/durable/temporal` (optional cluster).
- Go client SDK under `go/trajir/client` (open, project, seal, exec, commit, resume, RunStep).
- Dependabot for Go, pip, and Actions; CI hash golden gates and govulncheck; CONTRIBUTING Go section.
- Go kill mid deploy demo under `go/examples/kill-mid-deploy` using trajir/client.
- Dependabot DCO exclusion in CI; Actions and modernc.org/sqlite dependency bumps.
- Python `.tir` thin/fat export and import with node hash verification (R05).
- Security hardening: `.tir` zip size/path limits, atomic TOOL_CALL claim (Python + Go),
  identity delimiter validation, tenant-scoped list/export, redacted export mode,
  Temporal TLS/API key config, safer import verification API.
- CI: `pip-audit` gate in Python unit tests under CI (parity with Go `govulncheck`);
  `pip-audit` added to the `dev` extra.
- Go `.tir` thin/fat export and import (`trajir/tir`) with Python layout parity,
  hash verification, zip limits/path safety, and cross-language golden fixture.
- R05 conformance tests for `.tir` thin/fat round-trip and golden fixture load.
- R03 PURE recompute-on-resume: `requires_block_and_gate` / `RequiresBlockAndGate`,
  conformance + Go tests; only NON_IDEMPOTENT_WRITE is gated.
- R04 default context projector (`project_context` / `trajir/projector`) with
  CONSTRAINT+pinned budget safety and `BUDGET_IMPOSSIBLE` (RFC 8785 size metric).
- R06 sandbox mode (`RunMode.SANDBOX`) rejects NON_IDEMPOTENT_WRITE before side effects.
- R07 `graft_artifact_ref` / `trajir/graft` transfers artifact refs only (never THOUGHT).
- R08 projection redaction (`runtime/redact`, `trajir/redact`); shared with `.tir` redacted export.
- Maintainer release notes and process: `docs/RELEASE.md`, `docs/RELEASE_NOTES_0.1.0.md`.

### Changed

- CONTRIBUTING and infrastructure docs describe the CI gates that actually run.
- Lint cleanups for Ruff/Mypy on the core package and test harness.

## [v0.1.0-draft] - 2026-07-27

### Added

- `put_artifact` helper and optional `cas=` on thin `export_tir` / `import_tir` for fail closed rehydrate (issue #73).

- **Master Specification (`README.md`)**: The authoritative definition of Trajectory IR (Spec v0.2-draft).
- **Infrastructure Blueprint (`infrastructure.md`)**: Detailed DevOps rules targeting `local`, `server-s3`, and `k8s-fluid` profiles. Outlines the sharded CAS layout and fallback mechanisms.
- **Community Governance**: Added CNCF `CODE_OF_CONDUCT.md`.
- **Contribution Guidelines**: Added `CONTRIBUTING.md`, enforcing DCO sign-offs (`Signed-off-by`), Everything Claude Code (ECC) subagent suite integration, and governance accountability rules.
- **Security Policy**: Added `SECURITY.md`, detailing threat models specific to tool execution (Safety Boundary Bypasses, Seal Tampering, Cache Poisoning) and establishing GitHub Private Advisories for confidential reporting.
- **Quickstart Guide**: Added `QUICKSTART.md` for local prototyping (updated later to match real APIs).

### Changed

- **Session Storage Consolidation (formerly CLOOP)**: Folded CLOOP's session storage design (mutable short-term state, append-only event log, and sharded CAS artifact store) directly into Trajectory IR's unified database and storage schemas rather than maintaining a separate runtime project.
- **Declarative Memory Provisioning (formerly CAMI)**: Deferred Kubernetes declarative memory-provisioning claims and storage classes to an optional future phase without blocking core package portability or Phase 1A development.
- **Durable Execution Rebuild Strategy**: Delegated crash-safe step execution, lease/heartbeat coordination, and deterministic replay to hardened, pluggable third-party backends (**DBOS** and Restate) rather than rebuilding custom execution orchestration engines.
