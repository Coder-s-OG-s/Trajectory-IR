# Contributing to Trajectory IR

Thanks for helping. Trajectory IR is a portable semantic layer for agent runs
(seals, effect classes, `.tir`, honest resume). The root `README.md` is the
master specification.

Also read: [MAINTAINERS.md](MAINTAINERS.md), [GOVERNANCE.md](GOVERNANCE.md),
[docs/ROADMAP.md](docs/ROADMAP.md), [docs/SCOPE_AND_NON_GOALS.md](docs/SCOPE_AND_NON_GOALS.md),
[SECURITY.md](SECURITY.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md),
[AI_POLICY.md](AI_POLICY.md).

## 1. Spec before code

1. Read the root `README.md` (Phase 1B language priority is in §5 and §12.1).
2. Do not implement behavior that is not defined there.
3. Do not reimplement durable execution (retry, lease, custom crash engines).
   That belongs under `go/trajir/durable` (Temporal production for Go) or
   `drivers/durable_backend/` (DBOS for the Python reference port).
4. If something is ambiguous, open a **Spec question** issue and wait.

## 1.1 Phase 1B language rule (Go first)

Epic: [#113](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/113).

1. **New features, drivers, and demos** should land in **Go** first
   (`go/trajir`, `go/examples`), or dual language in the same PR.
2. **Python** is the **reference and parity port**. Keep CI green. Prefer
   Python only for Python specific fixes, DBOS work, or explicit parity
   follow ups named in the issue.
3. Do not open a Python only adoption demo or storage driver for Phase 1B
   work without an issue that says the Python port is intentional.
4. AI coding agents: default to Go for new Phase 1B work. Do not invent
   Python APIs when `go/trajir` already has the surface.

## 2. Developer Certificate of Origin (DCO)

Required on every commit:

```bash
git commit -s -m "feat: short description"
```

Pull requests with unsigned commits fail the **DCO** job in CI.

Dependabot bot commits are excluded from the DCO email match (the bot signs as
`support@github.com` but authors as `users.noreply.github.com`). Human commits
still require a matching `Signed-off-by`.

## 3. Pull requests

1. Prefer a linked issue (`Closes #N`).
2. Keep the change focused.
3. **New functionality must include tests in the same PR** (unit tests at
   minimum; add integration/conformance coverage where relevant). This is
   enforced indirectly by the coverage floor below, but do not treat the
   floor as the bar — add tests for the behavior you actually added.
4. Fill in the PR template.
5. Wait for CI to go green.
6. Set the **Milestone** on both the issue and the PR (see
   [docs/MILESTONES.md](docs/MILESTONES.md)). Right now prefer **Phase 1C harden
   and adopt**. Park signatures, Fluid, and SaaS under **Future deferred product**.

### Automated CI (what runs today)

Defined in `.github/workflows/ci.yml`, grouped into a **fast gate** (no live
services, aims to stay under ~10 minutes) and a **deep gate** (allowed to run
slower). Both groups run on every PR; both are meant to be required once
branch protection is on (see
[docs/maintainer-branch-protection.md](docs/maintainer-branch-protection.md)).

Fast gate:

| Check | What it does |
|-------|----------------|
| **DCO** | Every commit on the PR has `Signed-off-by` |
| **Quality** | install, Ruff, Mypy, hash goldens, unit tests with **coverage floor** (`PYTHON_COV_FAIL_UNDER`, default 80%) (Python 3.11 and 3.12) |
| **Package smoke** | `python -m build`, install the wheel into a clean venv, import smoke |
| **Security (pip-audit)** | `pip-audit --skip-editable` on the installed dependency tree |
| **Go** | hash goldens, `go/trajir/...` **coverage floor** (`GO_COV_FAIL_UNDER`, default 80%; excludes optional Temporal package), `go test ./...`, `govulncheck` |
| **Integration (Postgres)** | Live `PostgresNodeLog` against a Postgres 16 service, Python (`test/integration/test_postgres_live.py`) and Go (`go test ./trajir/postgres/...`) |
| **Integration (MinIO)** | Live `S3CAS` against MinIO, Python only for now (`test/integration/test_s3_minio_live.py`); Go's `trajir/cas.ObjectAPI` still only has `MemoryObjectAPI` fakes, so a live Go MinIO step needs a real S3 client adapter first (issue #129) |

Deep gate:

| Check | What it does |
|-------|----------------|
| **Conformance & E2E** | e2e crash/resume plus full `conformance/` R01-R08 (Python 3.11 and 3.12) |
| **Integration (Postgres)** | Live `PostgresNodeLog` against a Postgres 16 service (`test/integration/test_postgres_live.py`) |
| **Integration (MinIO)** | Live `S3CAS` against MinIO (`test/integration/test_s3_minio_live.py`) |

Local dependency audit (optional): `RUN_PIP_AUDIT=1 pytest test/unit/test_pip_audit.py -q` after `pip install -e ".[dev]"`. Under GitHub Actions the dedicated **Security (pip-audit)** job is authoritative.

#### Optional local integration services

**Preferred (full stack):** [docs/LIVE_INTEGRATION_DOCKER.md](docs/LIVE_INTEGRATION_DOCKER.md)
and root [`docker-compose.live.yml`](docker-compose.live.yml) (Postgres + MinIO + Temporal).

One-command smoke (when Docker Server is up):

```bash
./scripts/run_live_matrix.sh
# Windows: .\scripts\run_live_matrix.ps1
```

One-off containers (same env vars as the compose guide):

Postgres:

```bash
docker run -d --name trajir-pg \
  -e POSTGRES_USER=trajir -e POSTGRES_PASSWORD=trajir -e POSTGRES_DB=trajir \
  -p 5432:5432 postgres:16.6
export TRAJIR_DATABASE_URL=postgresql://trajir:trajir@localhost:5432/trajir
pip install -e ".[dev,postgres]"
pytest test/integration/test_postgres_live.py -q
```

MinIO:

```bash
docker run -d --name trajir-minio -p 9000:9000 \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z server /data
export TRAJIR_S3_ENDPOINT_URL=http://127.0.0.1:9000
export TRAJIR_S3_BUCKET=trajir
export AWS_ACCESS_KEY_ID=minioadmin
export AWS_SECRET_ACCESS_KEY=minioadmin
pip install -e ".[dev,s3]"
# create bucket once (Python one-liner or mc)
python -c "from drivers.s3.cas import build_s3_client_from_env; c=build_s3_client_from_env(); c.create_bucket(Bucket='trajir')"
pytest test/integration/test_s3_minio_live.py -q
```

Without these env vars, integration tests are skipped; unit fakes still cover offline CI Quality.

Coverage floors and required check names: [docs/maintainer-branch-protection.md](docs/maintainer-branch-protection.md).

Phase 1A inventory (shipped vs deferred): [docs/PHASE_1A_STATUS.md](docs/PHASE_1A_STATUS.md).

Cross language hash vectors live in `testdata/hash_vectors.json` and must stay
identical in Python (`test/unit/test_hash_vectors.py`) and Go
(`go/trajir/nodes`). Do not change digests without updating both sides in the
same PR.

`testdata/sample_thin.tir` is a thin package generated by
`scripts/gen_tir_fixture.py` (Python `export_tir`) and imported by
`go/trajir/tir.TestImportPythonGoldenFixture` (`tir.Import`), proving a
package written by the Python reference port loads and verifies node ids and
seals in Go. The **Go** CI job regenerates it fresh on every run rather than
trusting the committed copy. The reverse direction (Go `tir.Export` read back
by Python `load_tir`) is not wired up yet.

### Procedural review

Changes to effect classification or resume / block-and-gate need careful human
review (`pkg/trajectory_ir/effects/`, `pkg/trajectory_ir/resume/`, and the Go
packages under `go/trajir/effects` and `go/trajir/resume`).

If you use AI tools for a meaningful share of a change, say so in the PR. You
are still responsible for correctness against the spec. See
[AI_POLICY.md](AI_POLICY.md) for the full AI usage policy and enforcement
process.

## 4. Local setup (Go, primary for Phase 1B)

Go lives under `go/`. This is the default contributor path for new Phase 1B work.

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR/go
go test ./...
go test ./trajir/nodes -run TestHashVectors -count=1
go test ./conformance -count=1 -v
```

Optional Temporal (needs a local server and worker; not required for default tests):

```bash
# Full stack: docker compose -f docker-compose.live.yml up -d
# TEMPORAL_HOSTPORT=localhost:7233
go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
```

See `go/README.md` for package map, client usage, and backend choices
(LocalSQLite coding default, Temporal production target). Live stack:
[docs/LIVE_INTEGRATION_DOCKER.md](docs/LIVE_INTEGRATION_DOCKER.md).

## 5. Local setup (Python, reference port)

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
python -m venv .venv
# Windows: .\.venv\Scripts\activate
# Unix:    source .venv/bin/activate
pip install -U pip
pip install -e ".[dev]"
```

Useful commands:

```bash
ruff check pkg drivers client test conformance examples
ruff format pkg drivers client test conformance examples
mypy
pytest test/unit/test_hash_vectors.py -q
pytest test/unit -q
pytest test/e2e -q
pytest conformance/ -q
```

## 6. Issues

Use the issue forms when you can: bug report, feature, or spec question.

Community issues (opened by non-maintainers) are automatically labeled with `needs triage` for review.

Security issues go through private vulnerability reporting (`SECURITY.md`),
not public issues.

## 7. Maintainer note

Enable branch protection on `main` as described in
`docs/maintainer-branch-protection.md` so DCO, Quality, and Go checks are
required before merge.

