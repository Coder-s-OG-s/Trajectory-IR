# Contributing to Trajectory IR

Thanks for helping. Trajectory IR is a portable semantic layer for agent runs
(seals, effect classes, `.tir`, honest resume). The root `README.md` is the
master specification.

## 1. Spec before code

1. Read the root `README.md`.
2. Do not implement behavior that is not defined there.
3. Do not reimplement durable execution (retry, lease, custom crash engines).
   That belongs under `drivers/durable_backend/` (DBOS in Phase 1A).
4. If something is ambiguous, open a **Spec question** issue and wait.

## 2. Developer Certificate of Origin (DCO)

Required on every commit:

```bash
git commit -s -m "feat: short description"
```

Pull requests with unsigned commits fail the **DCO** job in CI.

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
| **Quality** | install, Ruff, Mypy on `pkg/trajectory_ir`, unit tests, e2e crash tests, R01/R02 |

Quality runs on Python **3.11** and **3.12**.

### Procedural review

Changes to effect classification or resume / block-and-gate need careful human
review (`pkg/trajectory_ir/effects/`, `pkg/trajectory_ir/resume/`).

If you use AI tools for a meaningful share of a change, say so in the PR. You
are still responsible for correctness against the spec.

## 4. Local setup

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
pytest test/unit -q
pytest test/e2e -q
pytest conformance/ -q
```

## 5. Issues

Use the issue forms when you can: bug report, feature, or spec question.

Security issues go through private vulnerability reporting (`SECURITY.md`),
not public issues.

## 6. Maintainer note

After CI is green on `main`, turn on branch protection as described in
`docs/maintainer-branch-protection.md`.
