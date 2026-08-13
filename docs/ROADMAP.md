# Roadmap

Public roadmap for Trajectory IR. Normative product rules remain in the root
`README.md`. This file is the **URL** intended for CNCF Sandbox form “Roadmap”
and for contributors planning work.

Milestones board: https://github.com/Coder-s-OG-s/Trajectory-IR/milestones

## One-line mission

Portable, hash-verifiable intermediate representation for agent execution
trajectories (seals, effects, `.tir`) on top of existing durable backends.

## Shipped

| Release / phase | Highlights |
|-----------------|------------|
| **Phase 1A / v0.1.x** | Python reference, conformance R01–R08 baseline, thin/fat `.tir`, DBOS local profile |
| **Phase 1B / v0.2.0** | Go primary SDK, Postgres NodeLog, S3 CAS, adoption demos, CI depth |
| **Phase 1C / v0.2.1** | Branch protection on `main`, live Docker docs/scripts, Release workflow assets |

## Near term (next 1–3 months)

| Track | Goals |
|-------|--------|
| **Phase CI/CD harden** | Scorecard, CodeQL, secret/workflow scan, SBOM, Go race; SHA-pin Actions; optional require gitleaks/actionlint on `main` ([docs/CI_HARDENING.md](CI_HARDENING.md) when present) |
| **CNCF Sandbox prep** | Process pack: MAINTAINERS, governance, this roadmap, adopters; do **not** apply until critical checklist is green ([CNCF_SANDBOX_APPLICATION_OUTLINE.md](CNCF_SANDBOX_APPLICATION_OUTLINE.md)) |
| **Adoption** | Keep Go QUICKSTART and demos green; interop notes for `.tir` export/import |
| **Quality** | Keep dual-language parity, coverage floors, live Postgres/MinIO CI |

## Medium term (3–9 months)

| Theme | Direction |
|-------|-----------|
| Interop demos | Documented path: produce `.tir` in one runtime, consume in another |
| Spec stability | Clarify package/compat policy as adoption grows |
| Community | Multi-org contributors, adopters list, optional TAG engagement |
| Security maturity | OpenSSF Best Practices badge progress, Scorecard improvements |

## Explicit non-goals (do not prioritize)

Aligned with root README §5 / Future milestone:

- Custom crash detection, retry, or lease engines (use Temporal / DBOS / Restate)
- Becoming “the” agent framework (LangGraph/CrewAI competitor)
- Competing on LTM recall quality (Mem0/Zep)
- Multi-tenant SaaS control plane
- Fluid/k8s-fluid productization as a core requirement
- Package digital signatures until scoped under Future ([#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149))

## Business / product separation

Trajectory IR is an **open source library and portable format**. It is not a
hosted multi-tenant product. Development and releases happen in public on this
repository under Apache-2.0. Any commercial use by contributors or companies
must treat this project as **upstream OSS**, not as a private product fork
that redefines the public API without process.

## How to influence the roadmap

1. Open a Spec question or feature issue.
2. Assign the correct milestone (not Future for in-scope work).
3. For scope expansion beyond non-goals, maintainers must update the root
   README (§14 process) before implementation.

## Related

- [CNCF_SANDBOX_APPLICATION_OUTLINE.md](CNCF_SANDBOX_APPLICATION_OUTLINE.md)
- [PHASE_1C_STATUS.md](PHASE_1C_STATUS.md)
- [MILESTONES.md](MILESTONES.md)
- [RELEASE.md](RELEASE.md)
