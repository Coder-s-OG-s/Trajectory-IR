# Trajectory IR v0.1.1

Patch release after **v0.1.0**: reliability and honesty fixes for the Phase 1A
library surface, CI Phase B live driver jobs, adoption demo and E2E docs, and
Go Temporal durable backend wording. No new Phase 2 scope.

## Highlights

- **CI Phase B**: live Postgres NodeLog and MinIO S3 CAS integration jobs
- **Adoption**: `examples/adoption_host` (public client + optional CAS thin package)
- **Docs**: E2E Postgres + CAS + thin `.tir` walkthrough
- **Fail closed storage**: missing artifact `content_hash` raises
- **Atomic `.tir` export**: no half written package at the destination path
- **Postgres claim honesty**: real DB errors are not misread as lost race
- **Client IR history**: plain (non gated) tools leave `TOOL_CALL` / `TOOL_RESULT`
- **Client `resume()`**: real reattach with history check, not a silent alias
- **Go / Python effect parity**: `openWorldHint` fails closed in Go
- **Projector safety**: `CONSTRAINT` always survives policy construction
- **Spec**: Temporal recognized as Go production durable backend (#67)

## Install (from source)

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
git checkout v0.1.1
pip install -e ".[dev]"
# optional: pip install -e ".[s3]" or ".[postgres]"
```

When PyPI is published:

```bash
pip install trajectory-ir==0.1.1
```

Go:

```bash
cd go && go test ./...
```

## What landed since v0.1.0

| Area | PRs (approx.) |
|------|----------------|
| Phase B CI (Postgres + MinIO) | #86 |
| Package / branch metadata | #84 |
| Go `openWorldHint` fail closed | #99 |
| Plain tool logging in client | #100 |
| CONSTRAINT always in projector policy | #101 |
| Durable test fake docs (Go) | #102 |
| Missing `content_hash` fail closed | #103 |
| NodeLog connection GC close | #104 |
| Atomic `.tir` write | #105 |
| Postgres `claim_tool_call` error propagation | #106 |
| Client `resume()` reattach | #107 |
| Adoption host demo | #108 |
| QUICKSTART E2E Postgres + CAS + thin | #109 |
| Release notes prep | #110 |
| Go Temporal backend wording in README | #112 |

## Not in 0.1.1

Same non goals as 0.1.0:

- Package digital signatures (`SIGNATURE` reserved)
- Live Restate cluster product packaging
- Fluid / k8s-fluid profile
- Multi tenant SaaS control plane
- Automated PyPI Trusted Publishing (manual upload still optional)
- Phase 1B Go primary program (tracked under epic #113; not part of this patch)

## Maintainer: after this release commit merges

1. Confirm CI green on the merge commit on `main`.
2. Annotated tag:

```bash
git checkout main
git pull origin main
git tag -a v0.1.1 -m "Trajectory IR v0.1.1 — post-0.1.0 reliability and Phase B CI"
git push origin v0.1.1
```

3. Publish a GitHub Release for `v0.1.1` using this file as the body.
4. Optional: build + `twine upload` for PyPI (`trajectory-ir==0.1.1`).

See [RELEASE.md](RELEASE.md).

## Full changelog

See [CHANGELOG.md](../CHANGELOG.md) section `[0.1.1]`.
