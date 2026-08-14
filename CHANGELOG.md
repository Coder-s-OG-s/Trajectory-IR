# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- MCP: in-tree Trajectory IR server (`go/trajir/mcp`, `go/cmd/trajir-mcp`) with
  tools `trajectory_status`, `trajectory_export_tir`, `trajectory_import_tir`,
  `trajectory_verify_signature`; docs in [docs/INTEGRATIONS.md](docs/INTEGRATIONS.md)
  and [integrations/mcp/README.md](integrations/mcp/README.md).
- Go: `trajir/tir` package signatures — `Sign` / `Verify`, optional
  `ExportOptions.SignKey`, Load verifies present `SIGNATURE` (README §9.1,
  [#177](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/177) / epic
  [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)). Golden
  vectors under `go/trajir/tir/testdata/sig_v1/` for Python parity (Phase C).
- Spec: package signature scheme **`trajir-pkg-sig-v1`** in README §9.1
  (payload digest, domain-separated Ed25519, file-only `SIGNATURE` member,
  unsigned default). Implementation remains Future ([#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)).
- Phase CI/CD harden: OpenSSF Scorecard, CodeQL (Go+Python), gitleaks,
  actionlint, zizmor advisory scan, Go race on core packages, CODEOWNERS,
  CycloneDX SBOM on release tags ([docs/CI_HARDENING.md](docs/CI_HARDENING.md)).
- CNCF Sandbox process pack: `MAINTAINERS.md`, `GOVERNANCE.md`, `docs/ROADMAP.md`,
  `ADOPTERS.md`, `docs/SCOPE_AND_NON_GOALS.md` (#172).

### Changed

- README §5 out-of-scope: signature **implementation** deferred; scheme is
  normative in §9.1. Proposed conformance R09–R11 listed for Future.
- Scope/roadmap/milestone docs: distinguish defined scheme vs implementation
  for [#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149).
- README project links; SECURITY.md supported versions for 0.2.x (#172).
- Pin GitHub Actions to commit digests with version comments (#167).
- Require `Secret scan (gitleaks)` and `Workflow lint (actionlint)` on `main` (#168).

### Fixed

## [0.2.1] - 2026-08-13

Phase 1C harden-and-adopt patch: merge gates on public `main`, live Docker
matrix tooling, and release workflow proof cut.

### Added

- Phase 1C: `docker-compose.live.yml` and `docs/LIVE_INTEGRATION_DOCKER.md` for
  local Postgres + MinIO + Temporal live matrix (#154, #157).
- Phase 1C: maintainer merge policy while protection was blocked (#152, #157).
- Phase 1C: classic branch protection on `main` with required CI checks
  (#146, #158, #163).
- Phase 1C: `scripts/run_live_matrix.sh` / `.ps1` for local live smoke (#160, #164).

### Changed

- Phase 1C status and milestones docs (#155, #161).
- Release process documents tag workflow asset verification (#153, #159).
- Go QUICKSTART demos and live stack links after first-success audit (#156).
- Live compose: drop broken `minio/mc` one-shot; create bucket via Python (#160).

### Fixed

## [0.2.0] - 2026-08-11

Phase 1B: Go is the primary SDK and onboarding path. Python remains the
reference and parity port. Includes Go drivers, adoption demo, CI depth, and
client honesty fixes on top of the 0.1.x line.

### Added

- Spec and process: Go primary for Phase 1B (README, CONTRIBUTING, go/QUICKSTART,
  PHASE_1B_STATUS) (issues #114, #116, #119, #133).
- Go adoption host demo with optional CAS thin package (issue #115).
- Go Postgres NodeLog (`trajir/postgres`) with offline sqlmock unit tests
  (issue #117).
- Go S3 compatible CAS (`ObjectAPI`) and AWS SDK v2 `NewS3StoreFromEnv`
  (issues #118, #134).
- CI: fast vs deep gates, Python↔Go `.tir` golden, Go coverage floor 80%,
  live Go Postgres in Integration (Postgres) (issues #128–#131, #129–#130).
- Milestone process docs (`docs/MILESTONES.md` when present).

### Changed

- Go client `Resume` requires existing NodeLog history (issue #132).
- Go client logs TOOL_CALL / TOOL_RESULT for non gated tools (parity with Python).

### Fixed

- govulncheck clean for pgx and aws-sdk-go-v2 S3 on the Phase 1B path.

## [0.1.1] - 2026-08-07

Patch release after `v0.1.0`: Phase B CI, reliability fixes, adoption docs and
demo, and Go Temporal durable backend wording. No Phase 2 scope expansion.

### Added

- CI Phase B: Integration (Postgres) and Integration (MinIO) jobs with live
  driver tests under `test/integration/` (issue #85, PR #86).
- Maintainer branch protection and free-plan notes expanded in
  `docs/maintainer-branch-protection.md`; richer PyPI package metadata in
  `pyproject.toml` (PR #84).
- Adoption host demo (`examples/adoption_host`) using the public Python client
  with optional filesystem CAS and thin `.tir` export (issue #87, PR #108).
- QUICKSTART / docs end to end walkthrough for Postgres NodeLog, CAS, and thin
  `.tir` (`docs/E2E_POSTGRES_CAS_THIN.md`) (issue #88, PR #109).
- Draft release notes and CHANGELOG prep path for this cut (PR #110).

### Changed

- `build_s3_client_from_env` uses path-style addressing when
  `TRAJIR_S3_ENDPOINT_URL` is set (MinIO/LocalStack) (PR #86).
- Go `trajir/durable` docs label `Memory` and `LocalSQLite` as durable backend
  **test fakes**, not production engines (issue #92 context, PR #102).
- Client SDK `resume()` is a documented reattach: requires existing node log
  history and fails loudly on empty trajectories instead of silently matching
  `open_trajectory` (issue #97, PR #107).
- Projector policy construction always keeps `CONSTRAINT` in
  `always_include_kinds` so policies cannot drop constraints by omission
  (PR #101).
- Master README and Go docs recognize Temporal as the production durable
  execution backend for the Go port alongside DBOS (Python) and Restate
  (optional) (issue #67, PR #112).

### Fixed

- Go effect classifier fails closed on `openWorldHint=true` (parity with Python
  `classify.py`) so open-world tools do not classify as `IDEMPOTENT_WRITE`
  (issue #98, PR #99).
- Client `exec_tool` appends `TOOL_CALL` / `TOOL_RESULT` for non-gated tools
  (PURE / READ_ONLY / IDEMPOTENT_WRITE), not only the block-and-gate path
  (issue #89, PR #100).
- `ensure_artifacts_in_cas` fails closed when a manifest entry is missing
  `content_hash` instead of skipping the row (PR #103).
- `NodeLog` closes its SQLite connection in `__del__` as a best-effort backstop
  for long-running client SDK sessions that never call `close()` (PR #104).
- `export_tir` writes packages atomically (temp file + fsync + `os.replace`) so
  a crash mid-export cannot leave a truncated `.tir` at the destination
  (PR #105).
- Postgres `claim_tool_call` propagates non-conflict errors; only
  `UniqueViolation` returns `False` for a lost race (issue #96, PR #106).

## [0.1.0] - 2026-08-06

First library-tagged release of the Phase 1A surface: dual-language IR, portable `.tir`, storage drivers, host example, Restate hooks, and CI Phase A gates (R01–R08).

### Added

- CI Phase A quality gates (issue #81): unit coverage floor (Python 80%), Go
  trajir coverage floor (50%), Package smoke (wheel build + import), dedicated
  Security (pip-audit) job; branch protection docs list required checks and
  "up to date with main".
- Agent host loop example (`examples/host_loop`) using only the public client
  SDK with a stub model and optional sandbox mode (issue #65).
- Restate durable backend adapter package (`drivers.durable_backend.restate`)
  with process local step memo; injectable durable hooks on `make_run_step`
  (issue #66).
- Go filesystem CAS under `go/trajir/cas` (issue #74).
- File based projector policy loader (YAML subset/JSON) and `policy=` on
  `project_context` (issue #75).
- Maintainer PyPI publish steps expanded in `docs/RELEASE.md` (issue #76).
- `put_artifact` helper and optional `cas=` on thin `export_tir` / `import_tir`
  for fail closed rehydrate (issue #73).
- Local filesystem content addressed store (`trajectory_ir.storage.FileSystemCAS`)
  with sharded `cas/<2-hex>/<hash>` layout, hash verify on get, and
  `rehydrate_artifacts` for thin `.tir` packages (issue #62).
- S3 compatible CAS driver (`drivers.s3.S3CAS`) with the same sharded key layout
  and injectable client for tests; optional `boto3` extra (issue #63).
- PostgreSQL NodeLog driver (`drivers.postgres.PostgresNodeLog`) (issue #64).
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

- Python `pip-audit` CI gate moved to the **Security (pip-audit)** job; unit helper
  remains for local `RUN_PIP_AUDIT=1`.
- CONTRIBUTING and infrastructure docs describe the CI gates that actually run.
- Lint cleanups for Ruff/Mypy on the core package and test harness.

### Fixed

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
