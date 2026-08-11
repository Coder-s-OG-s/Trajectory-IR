# Phase 1C status (harden and adopt)

Follow on after **v0.2.0** (Phase 1B Go primary). Focus: enforce quality on
merge, automate release cuts, and polish adoption without new Phase 2 product
scope.

Milestone: **Phase 1C harden and adopt**

## Goals

| Issue | Topic | Status |
|-------|--------|--------|
| #146 | Enable required status checks on `main` when plan allows | Open (blocked on private free GitHub: needs public or paid plan) |
| #147 | Release workflow on `v*` tags (wheel + release assets) | In progress with this doc PR when shipped |
| #148 | This status doc + maintainer checklist | This file |

## Done already on main (inherited)

- Green PR CI: Quality, Go, Package smoke, Security, Conformance, Integration Postgres/MinIO (Python + Go live paths)
- Go coverage floor 80% (unit set)
- Cross language `.tir` golden
- Tagged **v0.2.0** Phase 1B release

## Branch protection note

API returns HTTP 403 for branch protection on this private free repo. Until the
org upgrades or the repo is public, **policy** remains: only merge green PRs,
prefer squash, keep `main` up to date. Track unlock under #146.

## Maintainer checklist (post v0.2.0)

1. Confirm latest `main` CI is green.
2. Prefer milestone **Phase 1C** for new harden work; park signatures/Fluid/SaaS under **Future deferred product**.
3. When cutting a tag: `pyproject.toml` version, CHANGELOG section, annotated tag, GitHub Release notes.
4. After #147: verify tag push builds and attaches wheel/sdist.

## Related

- [PHASE_1B_STATUS.md](PHASE_1B_STATUS.md)
- [RELEASE.md](RELEASE.md)
- [RELEASE_NOTES_0.2.0.md](RELEASE_NOTES_0.2.0.md)
