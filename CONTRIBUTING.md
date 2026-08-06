# Contributing to Trajectory IR

Thanks for helping. Trajectory IR is a portable semantic layer for agent runs
(seals, effect classes, `.tir`, honest resume). The root `README.md` is the
master specification.

## 1. Spec before code

1. Read the root `README.md`.
2. Do not implement behavior that is not defined there.
3. Do not reimplement durable execution (retry, lease, custom crash engines).
   That belongs under `drivers/durable_backend/` (DBOS in Phase 1A) or
   `go/trajir/durable` (Go backends).
4. If something is ambiguous, open a **Spec question** issue and wait.

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
3. Fill in the PR template.
4. Wait for CI to go green.

### Automated CI (what runs today)

Defined in `.github/workflows/ci.yml`:

| Check | What it does |
|-------|----------------|
| **DCO** | Every commit on the PR has `Signed-off-by` |
| **Quality** | install, Ruff, Mypy, hash goldens, unit tests with **coverage floor** (`PYTHON_COV_FAIL_UNDER`, default 80%), e2e, full `conformance/` R01–R08 (Python 3.11 and 3.12) |
| **Package smoke** | `python -m build`, install the wheel into a clean venv, import smoke |
| **Security (pip-audit)** | `pip-audit --skip-editable` on the installed dependency tree |
| **Go** | hash goldens, `go/trajir/...` **coverage floor** (`GO_COV_FAIL_UNDER`, default 50%), `go test ./...`, `govulncheck` |

Local dependency audit (optional): `RUN_PIP_AUDIT=1 pytest test/unit/test_pip_audit.py -q` after `pip install -e ".[dev]"`. Under GitHub Actions the dedicated **Security (pip-audit)** job is authoritative.

Coverage floors and required check names: [docs/maintainer-branch-protection.md](docs/maintainer-branch-protection.md).

Phase 1A inventory (shipped vs deferred): [docs/PHASE_1A_STATUS.md](docs/PHASE_1A_STATUS.md).

Cross language hash vectors live in `testdata/hash_vectors.json` and must stay
identical in Python (`test/unit/test_hash_vectors.py`) and Go
(`go/trajir/nodes`). Do not change digests without updating both sides in the
same PR.

### Procedural review

Changes to effect classification or resume / block-and-gate need careful human
review (`pkg/trajectory_ir/effects/`, `pkg/trajectory_ir/resume/`, and the Go
packages under `go/trajir/effects` and `go/trajir/resume`).

If you use AI tools for a meaningful share of a change, say so in the PR. You
are still responsible for correctness against the spec.

## 4. Local setup (Python)

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
mypy pkg/trajectory_ir
pytest test/unit/test_hash_vectors.py -q
pytest test/unit -q
pytest test/e2e -q
pytest conformance/ -q
```

## 5. Local setup (Go)

Go lives under `go/`. Python remains the Phase 1A reference runtime.

```bash
cd go
go test ./...
go test ./trajir/nodes -run TestHashVectors -count=1
go test ./conformance -count=1 -v
```

Optional Temporal (needs a local server and worker; not required for default tests):

```bash
# TEMPORAL_HOSTPORT=localhost:7233
go test -tags=temporal_integration ./trajir/durable/temporal -count=1 -v
```

See `go/README.md` for package map, client usage, and backend choices
(LocalSQLite default, Temporal production target).

## 6. Issues

Use the issue forms when you can: bug report, feature, or spec question.

Security issues go through private vulnerability reporting (`SECURITY.md`),
not public issues.

## 7. Maintainer note

Enable branch protection on `main` as described in
`docs/maintainer-branch-protection.md` so DCO, Quality, and Go checks are
required before merge.
