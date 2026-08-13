# Governance

Trajectory IR is an open source project under the Apache License 2.0. This
document describes how decisions are made and how maintainership works.

## Principles

1. **Spec before code** — the root `README.md` is the master specification.
2. **Thin core** — do not reimplement durable execution engines (Temporal, DBOS, Restate).
3. **Portability first** — the portable `.tir` unit and effect-safe seals are the product.
4. **CNCF-friendly process** — DCO, public discussion, documented maintainers.

## Roles

| Role | Responsibility |
|------|----------------|
| **Maintainer** | Merge rights, release cuts, roadmap, security response, governance changes |
| **Contributor** | PRs, issues, reviews, docs; no merge rights |
| **User / adopter** | Feedback, bug reports, optional listing in `ADOPTERS.md` |

Current maintainers: [MAINTAINERS.md](MAINTAINERS.md).

## Decision making

1. **Day-to-day** — maintainers merge PRs that pass CI, match the spec, and have
   DCO. Prefer squash merge; keep `main` green.
2. **Spec or scope changes** — open a Spec question issue or PR that edits the
   root README / roadmap first. Wait for maintainer consensus (lazy consensus:
   silence of 5 business days after explicit call for objections may count as
   assent for non-breaking docs; breaking or product-scope changes need explicit
   approval from at least two maintainers when more than one is active).
3. **Security** — private coordinated disclosure per [SECURITY.md](SECURITY.md).
4. **Disputes** — escalate to a short maintainer discussion (issue or call);
   record the outcome in the issue.

## Becoming a maintainer

A contributor may be nominated when they have:

- Sustained contributions (code, docs, or reviews) over multiple months
- Demonstrated understanding of the spec and non-goals
- Responsive, respectful community behavior (Code of Conduct)

Nomination: existing maintainer opens an issue. Acceptance: majority of
current maintainers (or unanimous if only two). Update `MAINTAINERS.md` in the
same PR that grants rights.

## Emeritus

Maintainers who step back are moved to an Emeritus section in
`MAINTAINERS.md` (or removed with thanks) and lose merge rights. They may
return via the same nomination process.

## Releases

Maintainers cut version tags per [docs/RELEASE.md](docs/RELEASE.md). The
Release workflow attaches Python dist artifacts and SBOMs on `v*` tags.

## Code of Conduct

[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md). Violations are handled by maintainers
with private contact options listed in that document and SECURITY.md where
relevant.

## Changes to this document

Edits to `GOVERNANCE.md` require a PR and approval from at least one other
maintainer when more than one is listed.
