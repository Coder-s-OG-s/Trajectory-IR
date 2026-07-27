# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Boilerplate directory structure initialization for Phase 1A python components (`pkg/`, `client/`, `drivers/`, `conformance/`, `spec/`).
- GitHub workflow CI/CD documentation and DCO requirements.

## [v0.1.0-draft] - 2026-07-27
### Added
- **Master Specification (`README.md`)**: The authoritative definition of Trajectory IR (Spec v0.2-draft).
- **Infrastructure Blueprint (`infrastructure.md`)**: Detailed DevOps rules targeting `local`, `server-s3`, and `k8s-fluid` profiles. Outlines the sharded CAS layout and fallback mechanisms.
- **Community Governance**: Added CNCF `CODE_OF_CONDUCT.md`.
- **Contribution Guidelines**: Added `CONTRIBUTING.md`, enforcing DCO sign-offs (`Signed-off-by`) and AI Agent (ECC) accountability rules.
- **Security Policy**: Added `SECURITY.md`, detailing threat models specific to tool execution (Safety Boundary Bypasses, Seal Tampering, Cache Poisoning).
- **Quickstart Guide**: Added `QUICKSTART.md` for rapid prototyping using the embedded DBOS backend and SQLite/FileSystem CAS.

### Changed
- Shifted project scope away from rebuilding custom crash detection / memory storage (formerly CLOOP/CAMI) towards wrapping the DBOS/Restate durable execution backends. 
