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
| [#146](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/146) | Required status checks on `main` when plan allows | Open (blocked: private free GitHub 403) |
| [#152](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/152) | Maintainer merge policy while protection blocked | Open → this PR documents checklist |
| [#153](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/153) | Verify Release workflow attaches wheel/sdist on next `v*` tag | Open (workflow on main; v0.2.0 assets empty until next cut) |
| [#154](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/154) | Docker Compose live matrix (Postgres + MinIO + Temporal) | Open → `docker-compose.live.yml` + [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md) |
| [#155](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/155) | Refresh this status doc + milestones | Open → this file |
| [#156](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/156) | First-success Go QUICKSTART + demos audit | Open → go path re-verified green |
| [#147](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/147) | Release workflow on `v*` tags | **Closed** (workflow landed) |
| [#148](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/148) | Initial Phase 1C status scaffold | **Closed** |

## Done already on main (inherited)

- Green PR CI: Quality, Go, Package smoke, Security, Conformance, Integration Postgres/MinIO (Python + Go live paths)
- Go coverage floor 80% (unit set; Temporal package excluded from floor only)
- Cross language `.tir` golden
- Tagged **v0.2.0** Phase 1B release
- `.github/workflows/release.yml` on `v*` tags (build + attach dist)

## Branch protection note

API returns HTTP 403 for branch protection on this private free repo. Until the
org upgrades or the repo is public, **policy** remains: only merge green PRs,
prefer squash, keep `main` up to date. Permanent unlock: [#146](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/146).
Temporary checklist: [maintainer-branch-protection.md](maintainer-branch-protection.md)
(section **Merge policy while protection is blocked**).

## Maintainer checklist (post v0.2.0)

1. Confirm latest `main` CI is green.
2. Prefer milestone **Phase 1C** for new harden work; park signatures/Fluid/SaaS under **Future deferred product** ([#149](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/149)).
3. When cutting a tag: `pyproject.toml` version, CHANGELOG section, annotated tag, GitHub Release notes; confirm **Release** workflow attaches wheel + sdist ([RELEASE.md](RELEASE.md)).
4. Live services locally: [LIVE_INTEGRATION_DOCKER.md](LIVE_INTEGRATION_DOCKER.md).
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
