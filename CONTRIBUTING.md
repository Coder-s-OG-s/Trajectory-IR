# Contributing to Trajectory IR

First off, thank you for considering contributing to Trajectory IR! 

This document outlines the workflow and strict requirements for getting your code merged. Because Trajectory IR functions as an authoritative, portable master specification, we enforce high standards for code changes and documentation.

## 1. The Golden Rule: Spec Before Code

The `README.md` (and the technical specifications inside `spec/`) acts as the single source of truth. 
* **Do not** implement behavior that isn't defined in the spec.
* **Do not** derive undefined behavior from general familiarity with similar systems (e.g., Temporal, LangGraph). 
* If a design point is ambiguous, open an issue tagged `SPEC-QUESTION` and wait for a resolution. **Do not guess**.

## 2. Developer Certificate of Origin (DCO) Sign-off

**This is a hard requirement.** We enforce the DCO for all commits to ensure that contributors have the right to submit the code under the Apache-2.0 license.

Every single commit must contain the following trailer at the end of the commit message:

```text
Signed-off-by: Jane Doe <jane.doe@example.com>
```

You can add this automatically to your commits by using the `-s` or `--signoff` flag with git:

```bash
git commit -s -m "feat: add basic DBOS adapter framework"
```

**Pull Requests with unsigned commits will be automatically rejected by CI.**

## 3. Pull Request Process & Quality Gates

### A. AI Agent Working Agreements (ECC)
If you are an AI coding agent (like Claude Code or Antigravity), you are strictly bound by the rules in `README.md` Section 15. You must:
- Use the **Planner Agent** to create an implementation plan before writing core logic.
- Use the **TDD-Guide Agent** to enforce >80% test coverage.
- Use the **Security-Review Agent** for anything touching `pkg/effects/` or `pkg/resume/`.

### B. Conformance Testing
No PR will be merged if it violates the conformance tests defined in `conformance/`. 
- **R01 (Safe Resume)** and **R02 (Block-and-Gate)** are hard blockers for Phase 1A. If you modify the adapter interface or the resume logic, you must prove those tests still pass.

### C. Formatting and Linting
We use modern Python tooling:
- Run `ruff check .` to lint.
- Run `ruff format .` to format your code.
- Run `mypy .` to ensure strict type safety.

## 4. Setting up your environment

1. Clone the repository.
2. We recommend using **Hatch** (`pip install hatch`) to manage the Python environment.
3. Run `hatch shell` to enter the virtual environment.
4. Run `pytest` to run the existing test suite.

We look forward to reviewing your pull requests!
