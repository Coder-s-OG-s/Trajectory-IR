# Contributing to Trajectory IR

Thanks for helping. Trajectory IR is a portable semantic layer for agent runs
(seals, effect classes, `.tir`, honest resume). The root `README.md` is the
master specification. Code and docs should follow it.

## 1. Spec before code

1. Read the root `README.md` and the relevant files under `spec/`.
2. Do not implement behavior that is not defined there.
3. Do not reimplement durable execution (retry, lease, custom crash engines).
   That belongs to the backend adapter (`drivers/durable-backend/`), defaulting
   to DBOS in Phase 1A.
4. If something is ambiguous, open a **Spec question** issue and wait. Do not
   invent APIs, node kinds, or resume rules.

## 2. Developer Certificate of Origin (DCO)

Required on every commit under the Apache-2.0 license.

```bash
git commit -s -m "feat: short description"
```

That adds a trailer like:

```text
Signed-off-by: Your Name <you@example.com>
```

Use the same name and email as your GitHub account. Pull requests with unsigned
commits fail the **DCO** job in CI.

## 3. Pull requests

1. Prefer a linked issue (`Closes #N` or `Part of #2`).
2. Keep the change focused. Scaffold, process, and runtime work should not land
   in one giant PR without a good reason.
3. Fill in the PR template (summary, tests, safety, AI disclosure if needed).
4. Wait for CI to go green.

### Automated CI (what actually runs today)

Defined in `.github/workflows/ci.yml`:

| Check | What it does |
|-------|----------------|
| **DCO** | Every commit on the PR has `Signed-off-by` |
| **Quality** | `pip install -e ".[dev]"`, Ruff, Mypy on `pkg/trajectory_ir`, import smoke (`trajectory_ir`, `dbos`, `rfc8785`), `pytest` |

Python **3.11** and **3.12** both run the quality job.

### Conformance R01 / R02

R01 (safe resume) and R02 (block-and-gate) are the Phase 1A product gates from
the master README and issue #2. They are **not** wired into CI until those tests
exist under `conformance/`. When they land, they become required checks. Do not
treat them as already blocking merges today.

### Procedural review (humans + safety)

Changes to tool effect classification or resume / block-and-gate need careful
human review. Treat `pkg/trajectory_ir/effects/` and `pkg/trajectory_ir/resume/`
as high sensitivity once real logic exists there.

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
ruff check pkg test
ruff format pkg test
mypy pkg/trajectory_ir
pytest
```

Hatch is optional (`hatch shell`) if you prefer it; plain venv + pip is enough.

## 5. Issues

Use the issue forms when you can:

1. **Bug report** for broken behavior
2. **Feature or enhancement** for new work in scope
3. **Spec question** when the README or `spec/` is unclear

Security issues go through private vulnerability reporting (see `SECURITY.md`),
not public issues.

## 6. Maintainer note

After CI is green on `main`, turn on branch protection as described in
`docs/maintainer-branch-protection.md`.
