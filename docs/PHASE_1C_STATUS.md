# Phase 1C status (harden and adopt)

Follow on after **v0.2.0** (Phase 1B Go primary). Focus: enforce quality on
merge, prove release automation, deepen live integrations, and polish adoption
without new Phase 2 product scope.

Milestone: **[Phase 1C harden and adopt](https://github.com/Coder-s-OG-s/Trajectory-IR/milestone/5)**  
Epic: **[#151](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/151)**

## Goals

| Issue | Topic | Status |
|-------|--------|--------|
| [#151](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/151) | Epic: Phase 1C tracking | Open |
| [#146](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/146) | Required status checks on `main` | **Done** (protection applied; docs in #158 PR) |
| [#158](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/158) | Apply main branch protection + required CI | **Done** (API applied 2026-08-13) |
| [#152](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/152) | Maintainer merge policy while protection blocked | **Closed** (#157) |
| [#154](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/154) | Docker Compose live matrix docs | **Closed** (#157) |
| [#155](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/155) | Refresh status at #157 | **Closed** (#157) |
| [#156](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/156) | First-success Go path audit | **Closed** (#157) |
| [#147](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/147) | Release workflow on `v*` tags | **Closed** |
| [#148](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/148) | Initial Phase 1C status scaffold | **Closed** |
| [#159](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/159) | Prove Release attaches wheel/sdist on next tag | **In progress** (`v0.2.1` cut) |
| [#160](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/160) | Smoke live Docker stack + matrix scripts | **Closed** (#164) |
| [#161](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/161) | Refresh PHASE_1C_STATUS after #157 | **Closed** (#163) |
| [#162](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/162) | Close Phase 1C when exit criteria met | Open |

## Done already on main

- Green PR CI: Quality, Go, Package smoke, Security, Conformance, Integration Postgres/MinIO
- Go coverage floor 80% (unit set; Temporal package excluded from floor only)
- Cross language `.tir` golden
- Tagged **v0.2.0** Phase 1B release
- `.github/workflows/release.yml` on `v*` tags
- Live compose + docs (`docker-compose.live.yml`, [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md))
- **Branch protection on `main`** with strict required checks (see [maintainer-branch-protection.md](maintainer-branch-protection.md))

## Branch protection note

Repository is **public**. Classic protection is **enabled** on `main` with the
ten required check names listed in [maintainer-branch-protection.md](maintainer-branch-protection.md).
Force push and branch delete are denied. Temporary merge-only policy (#152) is
historical; keep good merge hygiene (squash, DCO, milestones).

## Maintainer checklist (post v0.2.0)

1. Confirm latest `main` / PR CI is green (required checks must pass to merge).
2. Prefer milestone **Phase 1C** for remaining harden work; park signatures/Fluid/SaaS under **Future** ([#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)).
3. When cutting a tag: version + CHANGELOG + annotated tag; confirm **Release** attaches wheel + sdist ([#159](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/159), [RELEASE.md](RELEASE.md)).
4. Live services: [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md) ([#160](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/160)).
5. First-success path: [go/QUICKSTART.md](../go/QUICKSTART.md).

## Explicitly not Phase 1C product

- Package signatures ([#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149))
- Fluid / multi-tenant SaaS / automated PyPI Trusted Publishing (Future milestone)

## Related

- [PHASE_1B_STATUS.md](PHASE_1B_STATUS.md)
- [MILESTONES.md](MILESTONES.md)
- [RELEASE.md](RELEASE.md)
- [RELEASE_NOTES_0.2.0.md](RELEASE_NOTES_0.2.0.md)
- [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md)
- [maintainer-branch-protection.md](maintainer-branch-protection.md)
