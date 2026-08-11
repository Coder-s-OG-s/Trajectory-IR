# Phase 1B status (Go primary SDK)

Honest inventory after Phase 1A / `v0.1.0`, the `v0.1.1` line, and the Phase 1B
landings on main. Normative detail: root `README.md` (Go primary, Python reference).

## Decision

| Surface | Phase 1A | Phase 1B |
|---------|----------|----------|
| Primary language | Python | **Go** |
| Python role | Primary | Reference / parity |
| Go durable production | Temporal (shipped) | Temporal (unchanged) |
| New features | Often Python first | **Go first** (or dual PR) |

Epic: [#113](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/113)

## Workstreams

| Issue | Topic | Status |
|-------|--------|--------|
| #114 | Spec: declare Go primary in README | **Shipped** (#120) |
| #115 | Go adoption host demo | **Shipped** (#123) |
| #116 | Go first QUICKSTART | **Shipped** (#122) |
| #117 | Go Postgres NodeLog | **Shipped** (#124) |
| #118 | Go S3 compatible CAS | **Shipped** (#125) |
| #119 | CONTRIBUTING Go first process | **Shipped** (#121) |
| #128 | CI unlock / fast vs deep / Go first | **Shipped** (workflow; branch protection still plan limited) |
| #129 | Go live Postgres and MinIO CI | **Shipped** |
| #130 | Python to Go `.tir` round trip CI | **Shipped** (#139) |
| #131 | Go coverage floor 70 then 80 | **Shipped** (70 in #140, 80 in #143) |
| #132 | Go Resume fails closed without history | **Shipped** (#141) |
| #133 | Refresh this status doc | **This change** |
| #134 | Real AWS/MinIO S3 client for Go CAS | **Shipped** (#142) |

## Already on main (Go)

- IR: nodes, log (SQLite), effects, resume, sandbox, graft, redact, projector
- Client SDK: `OpenTrajectory`, `Resume` (requires history), plain tool logging
- Demos: kill mid deploy, **adoption host** (`go/examples/adoption_host`)
- Filesystem CAS; S3 CAS with `ObjectAPI` and AWS SDK v2 `NewS3StoreFromEnv`
- Postgres NodeLog (`trajir/postgres`) with offline sqlmock unit tests
- `.tir` thin/fat; Python export → Go import golden in CI
- Temporal production durable backend; LocalSQLite/Memory test fakes
- CI: Go coverage floor **80%** (unit set excludes optional Temporal package)

## How to run (newcomers)

```bash
cd go
go test ./...
go run ./examples/adoption_host
go run ./examples/adoption_host -with-package
```

- [go/QUICKSTART.md](../go/QUICKSTART.md)
- [go/examples/adoption_host/README.md](../go/examples/adoption_host/README.md)
- Root [QUICKSTART.md](../QUICKSTART.md) (Go first, Python reference below)

## Explicitly not Phase 1B goals

- Deleting Python or dropping R01–R08 Python CI
- Package signatures, Fluid productization, multi tenant SaaS
- Rewriting DBOS out of the Python port
- Branch protection on private free GitHub plans (needs public or paid plan)

## Related

- Phase 1A inventory: [PHASE_1A_STATUS.md](PHASE_1A_STATUS.md)
- Milestones: [MILESTONES.md](MILESTONES.md) (when on main)
- Package version line: `0.1.1` in `pyproject.toml`
