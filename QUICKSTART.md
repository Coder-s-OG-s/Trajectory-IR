# Quickstart: Trajectory IR

Welcome to Trajectory IR! This guide will get you up and running with the `local` deployment profile in under 5 minutes. 

Trajectory IR acts as a semantic layer for AI agents, wrapping your agent's execution history in a portable, crash-safe format using pluggable durable execution backends like [DBOS](https://docs.dbos.dev/) or Restate.

## Prerequisites
- Python 3.11+
- `pip` or [Hatch](https://hatch.pypa.io/) (recommended)

---

## 1. Installation

### From this repository (Phase 1A development)

Until a release is published to PyPI, install from a local clone:

```bash
python -m venv .venv
# Windows: .\.venv\Scripts\activate
# Unix:    source .venv/bin/activate
pip install -U pip
pip install -e ".[dev]"
```

That pulls in the package plus Phase 1A deps (`dbos`, `rfc8785`) and dev tools (`pytest`, `ruff`, `mypy`, …).

Smoke check:

```bash
python -c "import trajectory_ir; print(trajectory_ir.__version__)"
python -c "from dbos import DBOS, DBOSConfig; print('DBOS import OK')"
python -c "import rfc8785; print(rfc8785.dumps({'b': 1, 'a': 2}))"
```

### From PyPI (later)

```bash
pip install trajectory-ir
```

`dbos` and `rfc8785` are declared dependencies of the package; you should not need to install them separately once a release is published.

## 2. Current Scope (Phase 1A scaffold)

Phase 1A only ships package scaffolding and dependency wiring. Runtime APIs and CLI commands (for init, tool/workflow decorators, sealing, and resume) are intentionally not implemented yet.

For now, use the install + smoke checks above to validate your environment.

## What's Next?
- Read the [Infrastructure Design](infrastructure.md) to learn how to scale this to `server-s3` or `k8s-fluid`.
- Read the [Contributing Guide](CONTRIBUTING.md) if you want to help build the project!
