# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Boilerplate directory structure initialization for Phase 1A python components (`pkg/`, `client/`, `drivers/`, `conformance/`, `spec/`).
- GitHub Actions CI (DCO, Ruff, Mypy, import smoke, pytest on Python 3.11/3.12).
- Issue templates (bug, feature, spec question) and pull request template.
- Maintainer notes for branch protection on `main`.
- Unit smoke tests under `test/unit/` so CI has a real pytest entrypoint.

### Changed
- CONTRIBUTING and infrastructure docs now describe the CI gates that actually run, and mark R01/R02 as future hard gates when conformance tests land.

## [v0.1.0-draft] - 2026-07-27
### Added
- **Master Specification (`README.md`)**: The authoritative definition of Trajectory IR (Spec v0.2-draft).
- **Infrastructure Blueprint (`infrastructure.md`)**: Detailed DevOps rules targeting `local`, `server-s3`, and `k8s-fluid` profiles. Outlines the sharded CAS layout and fallback mechanisms.
- **Community Governance**: Added CNCF `CODE_OF_CONDUCT.md`.
- **Contribution Guidelines**: Added `CONTRIBUTING.md`, enforcing DCO sign-offs (`Signed-off-by`), Everything Claude Code (ECC) subagent suite integration, and governance accountability rules.
- **Security Policy**: Added `SECURITY.md`, detailing threat models specific to tool execution (Safety Boundary Bypasses, Seal Tampering, Cache Poisoning) and establishing GitHub Private Advisories for confidential reporting.
- **Quickstart Guide**: Added `QUICKSTART.md` for rapid prototyping using the embedded DBOS backend and SQLite/FileSystem CAS behind an abstract `@Trajectory.workflow()` wrapper.

### Changed
- **Session Storage Consolidation (formerly CLOOP)**: Folded CLOOP's session storage design (mutable short-term state, append-only event log, and sharded CAS artifact store) directly into Trajectory IR's unified database and storage schemas rather than maintaining a separate runtime project.
- **Declarative Memory Provisioning (formerly CAMI)**: Deferred Kubernetes declarative memory-provisioning claims and storage classes to an optional future phase without blocking core package portability or Phase 1A development.
- **Durable Execution Rebuild Strategy**: Delegated crash-safe step execution, lease/heartbeat coordination, and deterministic replay to hardened, pluggable third-party backends (**DBOS** and Restate) rather than rebuilding custom execution orchestration engines.
