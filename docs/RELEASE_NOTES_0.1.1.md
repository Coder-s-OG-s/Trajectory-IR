# Trajectory IR v0.1.1 (draft)

Patch release after **v0.1.0**: reliability and honesty fixes for the Phase 1A
library surface, plus CI Phase B live driver jobs. No new Phase 2 scope.

**Status:** changelog and notes prepared on `main` (or the prep PR). Tag only
after `pyproject.toml` version is `0.1.1`, CHANGELOG section is renamed from
`[Unreleased]` to `[0.1.1] - YYYY-MM-DD`, and `main` is green.

## Highlights

- **CI Phase B**: live Postgres NodeLog and MinIO S3 CAS integration jobs
- **Fail closed storage**: missing artifact `content_hash` raises; thin export
  can verify CAS presence (already on 0.1.0 path, tightened here)
- **Atomic `.tir` export**: no half-written package at the destination path
- **Postgres claim honesty**: real DB errors are not misread as “lost the race”
- **Client IR history**: plain (non-gated) tools leave `TOOL_CALL` / `TOOL_RESULT`
- **Client `resume()`**: real reattach with history check, not a silent alias
- **Go / Python effect parity**: `openWorldHint` fails closed in Go
- **Projector safety**: `CONSTRAINT` always survives policy construction
- **Docs**: Go durable Memory / LocalSQLite called out as test fakes

## Install (from source)

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
git checkout v0.1.1   # after the tag exists
pip install -e ".[dev]"
# optional: pip install -e ".[s3]" or ".[postgres]"
```

When PyPI is published:

```bash
pip install trajectory-ir==0.1.1
```

## What landed since v0.1.0 (on main)

| Area | PRs (approx.) |
|------|----------------|
| Phase B CI (Postgres + MinIO) | #86 |
| Package / branch metadata | #84 |
| Go `openWorldHint` fail closed | #99 |
| Plain tool logging in client | #100 |
| CONSTRAINT always in projector policy | #101 |
| Durable test-fake docs (Go) | #102 |
| Missing `content_hash` fail closed | #103 |
| NodeLog connection GC close | #104 |
| Atomic `.tir` write | #105 |
| Postgres `claim_tool_call` error propagation | #106 |
| Client `resume()` reattach | #107 |

## Optional fold-ins before the tag

Merge these if you want them in the same patch (recommended for adopters):

| PR | Issue | Topic |
|----|-------|--------|
| #108 | #87 | Adoption host demo (public client + optional CAS thin package) |
| #109 | #88 | QUICKSTART / docs E2E: Postgres NodeLog, CAS, thin `.tir` |

After merge: add CHANGELOG lines under Added, re-run CI on `main`, then tag.

## Not in 0.1.1

Same non-goals as 0.1.0:

- Package digital signatures (`SIGNATURE` reserved)
- Live Restate cluster product packaging
- Fluid / k8s-fluid profile
- Multi-tenant SaaS control plane
- Automated PyPI Trusted Publishing (manual upload still optional)

## Spec note

Master README still names DBOS as the Phase 1A Python default; Go production
Temporal is implemented. Residual wording tracked in **issue #67** (owned
separately; not required to close for 0.1.1).

## How to cut the tag (maintainers)

See [RELEASE.md](RELEASE.md). Short form:

1. Merge any remaining fold-ins (#108 / #109 if desired).
2. Bump `pyproject.toml` → `version = "0.1.1"`.
3. Move CHANGELOG `[Unreleased]` body under `## [0.1.1] - <date>` and leave a
   fresh empty `[Unreleased]`.
4. Ensure CI is green on the release commit.
5. `git tag -a v0.1.1 -m "Trajectory IR v0.1.1 — post-0.1.0 reliability and Phase B CI"`
6. `git push origin v0.1.1` and publish a GitHub Release with this file as body.
7. Optional: build + `twine upload` for PyPI.

## Full changelog

See [CHANGELOG.md](../CHANGELOG.md) section `[Unreleased]` until the tag, then
`[0.1.1]`.
