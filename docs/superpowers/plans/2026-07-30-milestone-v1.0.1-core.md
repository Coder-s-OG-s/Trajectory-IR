# Milestone v1.0.1 - First Buildable Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a working, DBOS-backed Trajectory IR core where R01/R02 conformance tests pass against real crash-and-resume behavior, demonstrated end-to-end via a `kill -9` mid-deploy script.

**Architecture:** A content-addressed, append-only node log (SQLite) records every step of an agent trajectory. DBOS wraps model inference and tool calls as durable steps so a crash-resume never silently re-invokes either. Non-idempotent tool calls are additionally gated using the node log itself (not DBOS's internal status API) so a crash mid-effect blocks instead of auto-retrying.

**Tech Stack:** Python 3.11+, DBOS (embedded SQLite system DB), `rfc8785` for RFC 8785 JCS canonicalization, pytest, hatchling.

**Source spec:** [docs/superpowers/specs/2026-07-30-milestone-v1.0.1-design.md](../specs/2026-07-30-milestone-v1.0.1-design.md), tracking [issue #2](https://github.com/Coder-s-OG-s/Trajectory-IR/issues/2).

## Global Constraints

- Python >=3.11.
- Runtime deps: exactly `dbos`, `rfc8785`. Dev deps: `pytest`, `pytest-cov`, `ruff`, `mypy`. No others this milestone.
- Use `rfc8785` (Trail of Bits) for canonicalization — never `canonicaljson` (wrong RFC lineage).
- `ts` (wall-clock timestamp) must never be included in `payload_hash` input (spec §6.3) — assert this, don't just avoid it by convention.
- Model inference and tool calls must always go through `durable_infer`/`durable_tool` — never called raw inside a `@durable_workflow`-decorated function (spec §1 fix; this is the single most important convention in the codebase).
- `classify_from_mcp` must fail closed to `NON_IDEMPOTENT_WRITE` on any missing/ambiguous MCP annotation (spec §7.2).
- Client SDK exposes exactly 6 calls this milestone: `open_trajectory`, `project`, `seal_decision`, `exec_tool`, `commit_step`, `resume`. No 7th call.
- DBOS backend: embedded SQLite only this milestone. Postgres/S3/other `drivers/*` are scaffolded as empty packages, not implemented.
- Package directory is `drivers/durable_backend` (underscore) — the issue's own `mkdir` command shows a hyphen, but its import statements (`from drivers.durable_backend.dbos.adapter import ...`) require underscore since Python cannot import hyphenated paths. Underscore wins.
- Crash injection uses `Popen.kill()`, never `signal.SIGKILL` directly — Python's stdlib already maps `kill()` to SIGKILL on POSIX and `TerminateProcess` on Windows, so this is portable without platform branching and still a non-graceful hard kill.
- Before writing Task 6/7 code against DBOS's decorator and workflow-ID APIs, confirm current signatures against `docs.dbos.dev/python/tutorials/workflow-tutorial` and `docs.dbos.dev/python/reference/contexts` — the milestone issue explicitly flags this as unconfirmed and worth 5 minutes of reading rather than guessing, since R01/R02 depend on it.

---

### Task 1: Environment & project setup

**Files:**
- Create: `pyproject.toml`
- Modify: `.gitignore`

**Interfaces:**
- Produces: an installable `trajectory-ir` package exposing `trajectory_ir`, `drivers`, `client` as importable top-level packages.

- [ ] **Step 1: Write `pyproject.toml`**

```toml
[project]
name = "trajectory-ir"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = [
    "dbos",
    "rfc8785",
]

[project.optional-dependencies]
dev = ["pytest", "pytest-cov", "ruff", "mypy"]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["pkg/trajectory_ir", "drivers", "client"]

[tool.pytest.ini_options]
testpaths = ["test", "conformance"]
```

- [ ] **Step 2: Add build/runtime artifacts to `.gitignore`**

Append if not already present: `.venv/`, `*.sqlite`, `*.egg-info/`, `test_model_call_count.txt`, `test_deploy_side_effect_count.txt`, `*.marker`.

- [ ] **Step 3: Create venv and install**

```bash
python3 -m venv .venv
source .venv/bin/activate  # .venv\Scripts\activate on Windows
pip install --upgrade pip
pip install -e ".[dev]"
```

- [ ] **Step 4: Verify environment before writing project code**

```bash
python -c "from dbos import DBOS, DBOSConfig; print('DBOS OK')"
python -c "import rfc8785; print(rfc8785.dumps({'b':1,'a':2}))"
```

Expected: `DBOS OK` and `b'{"a":2,"b":1}'`. If either fails, stop and fix the environment before continuing — do not proceed to Task 2.

- [ ] **Step 5: Commit**

```bash
git add pyproject.toml .gitignore
git commit -s -m "build: add project dependencies and packaging config"
```

---

### Task 2: Repository scaffold

**Files:**
- Create: `pkg/trajectory_ir/__init__.py`, `pkg/trajectory_ir/runtime/__init__.py`, `pkg/trajectory_ir/resume/__init__.py`, `pkg/trajectory_ir/effects/__init__.py`, `pkg/trajectory_ir/package/__init__.py`
- Create: `drivers/__init__.py`, `drivers/durable_backend/__init__.py`, `drivers/durable_backend/dbos/__init__.py`, `drivers/sqlite/__init__.py`, `drivers/postgres/__init__.py`, `drivers/s3/__init__.py`
- Create: `client/__init__.py`, `client/python/__init__.py`
- Create empty directories: `conformance/`, `examples/kill-mid-deploy/`, `test/unit/`, `test/e2e/`, `docs/history/`, `spec/`

**Interfaces:**
- Produces: the directory/package skeleton every later task writes into.

- [ ] **Step 1: Create directories and `__init__.py` files**

```bash
mkdir -p spec pkg/trajectory_ir/{runtime,resume,effects,package} \
  drivers/durable_backend/dbos drivers/{sqlite,postgres,s3} \
  conformance examples/kill-mid-deploy test/unit test/e2e client/python docs/history

touch pkg/trajectory_ir/__init__.py \
  pkg/trajectory_ir/runtime/__init__.py \
  pkg/trajectory_ir/resume/__init__.py \
  pkg/trajectory_ir/effects/__init__.py \
  pkg/trajectory_ir/package/__init__.py \
  drivers/__init__.py drivers/durable_backend/__init__.py drivers/durable_backend/dbos/__init__.py \
  drivers/sqlite/__init__.py drivers/postgres/__init__.py drivers/s3/__init__.py \
  client/__init__.py client/python/__init__.py \
  conformance/__init__.py
```

`pkg/trajectory_ir/package/` and `drivers/{sqlite,postgres,s3}/` are scaffolded empty and stay that way this milestone (out of scope per design spec) — don't put code in them.

- [ ] **Step 2: Verify the package installs cleanly with the new layout**

```bash
pip install -e ".[dev]"
python -c "import trajectory_ir, drivers, client; print('scaffold OK')"
```

- [ ] **Step 3: Commit**

```bash
git add pkg drivers client conformance test docs/history spec .gitignore
git commit -s -m "chore: scaffold repository layout for milestone v1.0.1"
```

---

### Task 3: Node model + JCS hashing

**Files:**
- Create: `pkg/trajectory_ir/runtime/nodes.py`
- Test: `test/unit/test_node_identity.py`

**Interfaces:**
- Produces: `NODE_KINDS: frozenset[str]`, `payload_hash(payload: dict) -> str`, `node_id(tenant_id, trajectory_id, step_n, seq, kind, phash) -> str`, `Node` dataclass with fields `kind, trajectory_id, tenant_id, step_n, seq, payload, ts` and computed attributes `.phash`, `.id`.

- [ ] **Step 1: Write the failing tests**

```python
# test/unit/test_node_identity.py
import time

import pytest

from trajectory_ir.runtime.nodes import Node


def test_identical_payload_different_key_order_same_hash():
    n1 = Node(kind="STATE_SET", trajectory_id="t1", tenant_id="demo", step_n=1, seq=1, payload={"a": 1, "b": 2})
    n2 = Node(kind="STATE_SET", trajectory_id="t1", tenant_id="demo", step_n=1, seq=1, payload={"b": 2, "a": 1})
    assert n1.id == n2.id  # whole point of JCS


def test_ts_never_affects_hash():
    n1 = Node(kind="STATE_SET", trajectory_id="t1", tenant_id="demo", step_n=1, seq=1, payload={"a": 1})
    time.sleep(1.1)
    n2 = Node(kind="STATE_SET", trajectory_id="t1", tenant_id="demo", step_n=1, seq=1, payload={"a": 1})
    assert n1.id == n2.id  # ts differs, id must not


def test_unknown_kind_rejected():
    with pytest.raises(AssertionError):
        Node(kind="NOT_A_KIND", trajectory_id="t1", tenant_id="demo", step_n=1, seq=1, payload={})


def test_ts_key_in_payload_rejected():
    with pytest.raises(AssertionError):
        Node(kind="STATE_SET", trajectory_id="t1", tenant_id="demo", step_n=1, seq=1, payload={"ts": 123})
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest test/unit/test_node_identity.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'trajectory_ir.runtime.nodes'`.

- [ ] **Step 3: Implement**

```python
# pkg/trajectory_ir/runtime/nodes.py
import hashlib
import time
from dataclasses import dataclass, field
from typing import Optional

import rfc8785

NODE_KINDS = frozenset({
    "INPUT", "CONSTRAINT", "STATE_SET", "PROJECT_CONTEXT", "THOUGHT",
    "DECISION", "TOOL_CALL", "TOOL_RESULT", "ARTIFACT_PUT", "ARTIFACT_REF",
    "LTM_QUERY", "LTM_HIT", "LTM_PROJECT", "COMMIT_STEP", "ABORT", "REDACTION",
})


def payload_hash(payload: dict) -> str:
    """RFC 8785 canonicalize, then SHA-256. `ts` must never be hashed (spec §6.3)."""
    assert "ts" not in payload, "wall-clock ts must never be hashed (spec §6.3)"
    canon = rfc8785.dumps(payload)
    return hashlib.sha256(canon).hexdigest()


def node_id(tenant_id: str, trajectory_id: str, step_n: Optional[int], seq: int, kind: str, phash: str) -> str:
    raw = f"{tenant_id}|{trajectory_id}|{step_n}|{seq}|{kind}|{phash}".encode()
    return hashlib.sha256(raw).hexdigest()


@dataclass
class Node:
    kind: str
    trajectory_id: str
    tenant_id: str
    step_n: Optional[int]
    seq: int
    payload: dict
    ts: float = field(default_factory=time.time)

    def __post_init__(self):
        assert self.kind in NODE_KINDS, f"unknown node kind: {self.kind}"
        self.phash = payload_hash(self.payload)
        self.id = node_id(
            self.tenant_id, self.trajectory_id, self.step_n, self.seq, self.kind, self.phash
        )
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest test/unit/test_node_identity.py -v
```

Expected: 4 passed.

- [ ] **Step 5: Commit**

```bash
git add pkg/trajectory_ir/runtime/nodes.py
git commit -s -m "feat: add content-addressed Node model with RFC 8785 hashing"
git add test/unit/test_node_identity.py
git commit -s -m "test: verify node identity is order-independent and ts-invariant"
```

---

### Task 4: SQLite-backed append-only NodeLog

**Files:**
- Create: `pkg/trajectory_ir/runtime/log.py`
- Test: `test/unit/test_node_log.py`

**Interfaces:**
- Consumes: `Node` from `trajectory_ir.runtime.nodes` (Task 3).
- Produces: `NodeLog(db_path: str)` with `.append(kind, step_n, payload, trajectory_id, tenant_id, seq) -> Node`, `.has(trajectory_id, step_n, kind) -> bool`, `.count(node_id) -> int`, `.close()`.

- [ ] **Step 1: Write the failing tests**

```python
# test/unit/test_node_log.py
import os
import tempfile

import pytest

from trajectory_ir.runtime.log import NodeLog


@pytest.fixture
def log():
    fd, path = tempfile.mkstemp(suffix=".sqlite")
    os.close(fd)
    node_log = NodeLog(path)
    yield node_log
    node_log.close()
    os.remove(path)


def test_append_then_has(log):
    log.append("DECISION", step_n=1, payload={"plan": "x"}, trajectory_id="t1", tenant_id="demo", seq=1)
    assert log.has("t1", 1, "DECISION")
    assert not log.has("t1", 1, "TOOL_RESULT")


def test_append_is_idempotent_by_content(log):
    n1 = log.append("DECISION", step_n=1, payload={"plan": "x"}, trajectory_id="t1", tenant_id="demo", seq=1)
    n2 = log.append("DECISION", step_n=1, payload={"plan": "x"}, trajectory_id="t1", tenant_id="demo", seq=1)
    assert n1.id == n2.id
    assert log.count(n1.id) == 1
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest test/unit/test_node_log.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'trajectory_ir.runtime.log'`.

- [ ] **Step 3: Implement**

```python
# pkg/trajectory_ir/runtime/log.py
import json
import sqlite3
from typing import Optional

from trajectory_ir.runtime.nodes import Node


class NodeLog:
    """Append-only, content-addressed node log backed by SQLite.

    Appends are idempotent: replaying an append for a node whose id already
    exists is a no-op (`INSERT OR IGNORE`). This is what makes DBOS's
    workflow replay safe to layer on top of -- re-running an already-appended
    step produces the same node id and is silently absorbed instead of
    duplicating history, so there is no separate "seal" operation needed.
    """

    def __init__(self, db_path: str):
        self._conn = sqlite3.connect(db_path)
        self._conn.execute(
            """
            CREATE TABLE IF NOT EXISTS nodes (
                id TEXT PRIMARY KEY,
                trajectory_id TEXT NOT NULL,
                tenant_id TEXT NOT NULL,
                step_n INTEGER,
                seq INTEGER NOT NULL,
                kind TEXT NOT NULL,
                payload_json TEXT NOT NULL,
                ts REAL NOT NULL
            )
            """
        )
        self._conn.commit()

    def append(self, kind: str, step_n: Optional[int], payload: dict, trajectory_id: str, tenant_id: str, seq: int) -> Node:
        node = Node(
            kind=kind, trajectory_id=trajectory_id, tenant_id=tenant_id,
            step_n=step_n, seq=seq, payload=payload,
        )
        self._conn.execute(
            "INSERT OR IGNORE INTO nodes (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (node.id, node.trajectory_id, node.tenant_id, node.step_n, node.seq, node.kind, json.dumps(node.payload), node.ts),
        )
        self._conn.commit()
        return node

    def has(self, trajectory_id: str, step_n: int, kind: str) -> bool:
        cur = self._conn.execute(
            "SELECT 1 FROM nodes WHERE trajectory_id = ? AND step_n = ? AND kind = ? LIMIT 1",
            (trajectory_id, step_n, kind),
        )
        return cur.fetchone() is not None

    def count(self, node_id: str) -> int:
        cur = self._conn.execute("SELECT COUNT(*) FROM nodes WHERE id = ?", (node_id,))
        return cur.fetchone()[0]

    def close(self):
        self._conn.close()
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest test/unit/test_node_log.py -v
```

Expected: 2 passed.

- [ ] **Step 5: Commit**

```bash
git add pkg/trajectory_ir/runtime/log.py
git commit -s -m "feat: add SQLite-backed idempotent append-only NodeLog"
git add test/unit/test_node_log.py
git commit -s -m "test: verify NodeLog append idempotency by content-addressed id"
```

---

### Task 5: Effect classes + MCP mapping

**Files:**
- Create: `pkg/trajectory_ir/effects/classify.py`
- Modify: `pkg/trajectory_ir/effects/__init__.py`
- Create: `pkg/trajectory_ir/runtime/tool.py`
- Test: `test/unit/test_effects.py`

**Interfaces:**
- Produces: `EffectClass` enum (`PURE, READ_ONLY, IDEMPOTENT_WRITE, NON_IDEMPOTENT_WRITE, AGENT_SPAWN, SENSITIVE`), `classify_from_mcp(annotations: dict) -> EffectClass`, importable as `from trajectory_ir.effects import EffectClass, classify_from_mcp`. Also `Tool` dataclass (`name: str, fn: Callable, effect_class: EffectClass`) used by Task 7/8.

- [ ] **Step 1: Write the failing tests**

```python
# test/unit/test_effects.py
from trajectory_ir.effects import EffectClass, classify_from_mcp


def test_missing_annotations_fail_closed():
    assert classify_from_mcp({}) == EffectClass.NON_IDEMPOTENT_WRITE


def test_ambiguous_annotations_fail_closed():
    assert classify_from_mcp({"readOnlyHint": False}) == EffectClass.NON_IDEMPOTENT_WRITE


def test_read_only_hint_classified_read_only():
    assert classify_from_mcp({"readOnlyHint": True}) == EffectClass.READ_ONLY


def test_explicit_idempotent_write_classified_correctly():
    annotations = {"readOnlyHint": False, "idempotentHint": True, "destructiveHint": False}
    assert classify_from_mcp(annotations) == EffectClass.IDEMPOTENT_WRITE


def test_destructive_hint_true_fails_closed_even_if_idempotent():
    annotations = {"readOnlyHint": False, "idempotentHint": True, "destructiveHint": True}
    assert classify_from_mcp(annotations) == EffectClass.NON_IDEMPOTENT_WRITE
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest test/unit/test_effects.py -v
```

Expected: FAIL with `ImportError: cannot import name 'EffectClass'`.

- [ ] **Step 3: Implement**

```python
# pkg/trajectory_ir/effects/classify.py
from enum import Enum


class EffectClass(Enum):
    PURE = "PURE"
    READ_ONLY = "READ_ONLY"
    IDEMPOTENT_WRITE = "IDEMPOTENT_WRITE"
    NON_IDEMPOTENT_WRITE = "NON_IDEMPOTENT_WRITE"
    AGENT_SPAWN = "AGENT_SPAWN"
    SENSITIVE = "SENSITIVE"


def classify_from_mcp(annotations: dict) -> EffectClass:
    """Fail-closed per spec §7.2. Any missing or ambiguous annotation -> NON_IDEMPOTENT_WRITE."""
    if annotations.get("readOnlyHint") is True:
        return EffectClass.READ_ONLY
    if (
        annotations.get("readOnlyHint") is False
        and annotations.get("idempotentHint") is True
        and annotations.get("destructiveHint") is False
    ):
        return EffectClass.IDEMPOTENT_WRITE
    return EffectClass.NON_IDEMPOTENT_WRITE  # fail closed, always
```

```python
# pkg/trajectory_ir/effects/__init__.py
from trajectory_ir.effects.classify import EffectClass, classify_from_mcp

__all__ = ["EffectClass", "classify_from_mcp"]
```

```python
# pkg/trajectory_ir/runtime/tool.py
from dataclasses import dataclass
from typing import Callable

from trajectory_ir.effects import EffectClass


@dataclass
class Tool:
    name: str
    fn: Callable
    effect_class: EffectClass
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest test/unit/test_effects.py -v
```

Expected: 5 passed.

- [ ] **Step 5: Commit**

```bash
git add pkg/trajectory_ir/effects/classify.py pkg/trajectory_ir/effects/__init__.py pkg/trajectory_ir/runtime/tool.py
git commit -s -m "feat: add fail-closed EffectClass classification from MCP annotations"
git add test/unit/test_effects.py
git commit -s -m "test: verify effect classification fails closed on ambiguous annotations"
```

---

### Task 6: DBOS durable-backend adapter

**Files:**
- Create: `drivers/durable_backend/dbos/adapter.py`
- Test: `test/unit/test_dbos_adapter.py`

**Interfaces:**
- Produces: `init_backend(app_name: str = "trajectory-ir-local") -> None`, `durable_infer(fn) -> Callable`, `durable_tool(fn) -> Callable`, `durable_workflow(fn) -> Callable`.

- [ ] **Step 1: Confirm DBOS API surface against current docs**

Before writing code, check `docs.dbos.dev/python/tutorials/workflow-tutorial` and `docs.dbos.dev/python/reference/decorators` to confirm `DBOS.step()`, `DBOS.workflow()`, `DBOS(config=...)`, and `DBOS.launch()` are still the current names/signatures. This is a 5-minute check, not optional — the milestone issue flags this API as the biggest unknown in the build, and R01/R02 depend on it being right.

- [ ] **Step 2: Write the failing test**

```python
# test/unit/test_dbos_adapter.py
from drivers.durable_backend.dbos.adapter import durable_infer, durable_tool, durable_workflow, init_backend


def test_wrapped_workflow_runs_and_returns_result(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    init_backend(app_name="test-adapter")

    call_count = {"n": 0}

    def model_call(x):
        call_count["n"] += 1
        return x * 2

    infer = durable_infer(model_call)

    @durable_workflow
    def workflow(x):
        return infer(x)

    result = workflow(5)
    assert result == 10
    assert call_count["n"] == 1
```

- [ ] **Step 3: Run test to verify it fails**

```bash
pytest test/unit/test_dbos_adapter.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'drivers.durable_backend.dbos.adapter'`.

- [ ] **Step 4: Implement**

```python
# drivers/durable_backend/dbos/adapter.py
import os

from dbos import DBOS, DBOSConfig


def init_backend(app_name: str = "trajectory-ir-local") -> None:
    config: DBOSConfig = {
        "name": app_name,
        "system_database_url": os.environ.get(
            "DBOS_SYSTEM_DATABASE_URL", f"sqlite:///{app_name}.sqlite"
        ),
    }
    DBOS(config=config)
    DBOS.launch()


# Model inference MUST be wrapped identically to tool calls (spec §1 fix):
# without this, DBOS replays the whole workflow body on crash-resume and the
# model is silently re-invoked even though its output is discarded once
# execution reaches the memoized DECISION step. Do not simplify this away.
def durable_infer(fn):
    return DBOS.step()(fn)


# Tool calls MUST go through this wrapper -- never invoked raw inside a
# @durable_workflow-decorated function.
def durable_tool(fn):
    return DBOS.step()(fn)


def durable_workflow(fn):
    return DBOS.workflow()(fn)
```

- [ ] **Step 5: Run test to verify it passes**

```bash
pytest test/unit/test_dbos_adapter.py -v
```

Expected: 1 passed.

- [ ] **Step 6: Commit**

```bash
git add drivers/durable_backend/dbos/adapter.py
git commit -s -m "feat: add DBOS durable-backend adapter wrapping inference and tool calls"
git add test/unit/test_dbos_adapter.py
git commit -s -m "test: verify durable-wrapped workflow executes and returns correctly"
```

---

### Task 7: Seal/resume workflow, block-and-gate, and the kill-mid-deploy fixture agent

This is the hardest part of the milestone — where the §1 fix and crash-safety guarantees actually get enforced in code, not just documented. Budget the most time here; don't compress it.

**Files:**
- Create: `pkg/trajectory_ir/resume/gate.py`
- Create: `pkg/trajectory_ir/resume/step.py`
- Create: `examples/kill-mid-deploy/agent.py`
- Create: `test/e2e/_util.py`
- Test: `test/e2e/test_seal_resume_crash.py`

**Interfaces:**
- Consumes: `NodeLog` (Task 4), `EffectClass`/`Tool` (Task 5), `durable_infer`/`durable_tool`/`durable_workflow` (Task 6).
- Produces: `BlockedNeedsGate` exception, `make_gated_tool_call(node_log, trajectory_id, tenant_id, step_n, seq, tool_name, tool_fn) -> Callable`, `make_run_step(node_log, tenant_id, trajectory_id, tool_registry, on_decision_sealed=None) -> run_step(step_n, model_call, context) -> list`. `examples/kill-mid-deploy/agent.py` is a CLI reused by Task 9/10's conformance tests and Task 11's demo — build it once here.

- [ ] **Step 1: Write `pkg/trajectory_ir/resume/gate.py`**

```python
# pkg/trajectory_ir/resume/gate.py
class BlockedNeedsGate(Exception):
    """Raised when a NON_IDEMPOTENT_WRITE tool call was interrupted mid-
    execution and must not be silently retried. Resolving the block (manual
    replay/approval) is out of scope for this milestone -- raising is the
    gate."""

    def __init__(self, step_n: int, tool_name: str):
        self.step_n = step_n
        self.tool_name = tool_name
        super().__init__(
            f"step {step_n}: '{tool_name}' BLOCKED_NEEDS_GATE (crashed mid-execution, not retried)"
        )


def make_gated_tool_call(node_log, trajectory_id, tenant_id, step_n, seq, tool_name, tool_fn):
    """Wrap a NON_IDEMPOTENT_WRITE tool so a crash between 'started' and
    'completed' blocks instead of silently re-running the side effect on
    resume.

    Uses our own content-addressed NodeLog as the source of truth for "was
    this call started but never finished," rather than DBOS's internal
    workflow-status API -- this avoids depending on an internal API that may
    change between DBOS versions, at the cost of one extra durable log write
    before the effect runs.
    """

    def gated(**kwargs):
        if node_log.has(trajectory_id, step_n, "TOOL_CALL") and not node_log.has(trajectory_id, step_n, "TOOL_RESULT"):
            node_log.append(
                "ABORT", step_n, {"reason": "BLOCKED_NEEDS_GATE", "tool": tool_name},
                trajectory_id, tenant_id, seq,
            )
            raise BlockedNeedsGate(step_n, tool_name)

        node_log.append(
            "TOOL_CALL", step_n, {"tool": tool_name, "args": kwargs},
            trajectory_id, tenant_id, seq,
        )
        result = tool_fn(**kwargs)
        node_log.append(
            "TOOL_RESULT", step_n, {"result": result},
            trajectory_id, tenant_id, seq + 1,
        )
        return result

    return gated
```

- [ ] **Step 2: Write `pkg/trajectory_ir/resume/step.py`**

```python
# pkg/trajectory_ir/resume/step.py
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import make_gated_tool_call
from drivers.durable_backend.dbos.adapter import durable_infer, durable_tool, durable_workflow


def make_run_step(node_log, tenant_id, trajectory_id, tool_registry, on_decision_sealed=None):
    @durable_workflow
    def run_step(step_n: int, model_call, context: dict):
        node_log.append("PROJECT_CONTEXT", step_n, context, trajectory_id, tenant_id, seq=0)

        # Model inference wrapped as a durable step -- fix from spec §1. Without
        # durable_infer here, a crash after this line but before the DECISION
        # is sealed would cause DBOS to re-invoke model_call on resume even
        # though its output would be discarded once replay reaches DECISION.
        infer = durable_infer(model_call)
        plan = infer(context)

        # append() is idempotent by content, so this doubles as the "seal":
        # replaying it after a crash produces the same node id and is a no-op.
        node_log.append("DECISION", step_n, {"plan": plan}, trajectory_id, tenant_id, seq=1)
        if on_decision_sealed is not None:
            on_decision_sealed()

        results = []
        for i, call in enumerate(plan["tool_calls"]):
            tool = tool_registry[call["name"]]
            seq = 2 + i
            if tool.effect_class == EffectClass.NON_IDEMPOTENT_WRITE:
                gated = make_gated_tool_call(
                    node_log, trajectory_id, tenant_id, step_n, seq, call["name"], tool.fn
                )
                result = durable_tool(gated)(**call["args"])
            else:
                result = durable_tool(tool.fn)(**call["args"])
            results.append(result)

        node_log.append("COMMIT_STEP", step_n, {}, trajectory_id, tenant_id, seq=2 + len(plan["tool_calls"]))
        return results

    return run_step
```

- [ ] **Step 3: Confirm DBOS's workflow-ID pinning API**

The fixture agent below needs the *same* DBOS workflow id across the initial run and the `--resume` run, so DBOS recognizes it as one durable workflow to replay rather than a fresh one. Confirm the current API (as of writing: a `SetWorkflowID` context manager) against `docs.dbos.dev/python/reference/contexts` and adjust Step 4 below if it has changed.

- [ ] **Step 4: Write the fixture agent**

```python
# examples/kill-mid-deploy/agent.py
"""Kill-mid-deploy fixture agent.

Runs one durable step: infer a plan, seal the decision, execute a fake
`deploy_server` tool. Used by test/e2e, conformance/, and the kill-mid-deploy
demo -- one script, three consumers, so crash-recovery behavior is only
implemented once.
"""
import argparse
import os
import sys
import time

from dbos import SetWorkflowID

from drivers.durable_backend.dbos.adapter import init_backend
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import BlockedNeedsGate
from trajectory_ir.resume.step import make_run_step

TRAJECTORY_ID = "kill-mid-deploy-demo"
TENANT_ID = "demo"
DB_PATH = "kill_mid_deploy.sqlite"
MODEL_CALL_COUNT_FILE = "test_model_call_count.txt"
DEPLOY_COUNT_FILE = "test_deploy_side_effect_count.txt"
DECISION_SEALED_MARKER = "decision_sealed.marker"
TOOL_STARTED_MARKER = "tool_started.marker"


def _bump_counter(path: str) -> int:
    n = int(open(path).read().strip() or "0") if os.path.exists(path) else 0
    n += 1
    with open(path, "w") as f:
        f.write(str(n))
    return n


def model_call(context: dict) -> dict:
    _bump_counter(MODEL_CALL_COUNT_FILE)
    return {"tool_calls": [{"name": "deploy_server", "args": {"version": "1.0.0"}}]}


def deploy_server(version: str, crash_during: bool = False) -> dict:
    with open(TOOL_STARTED_MARKER, "w") as f:
        f.write("started")
    print("TOOL_CALL: deploy_server started", flush=True)
    if crash_during:
        time.sleep(5)  # give the external harness a window to hard-kill us
    _bump_counter(DEPLOY_COUNT_FILE)
    return {"deployed": version}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--resume", action="store_true")
    parser.add_argument("--crash-after", choices=["decision_sealed"], default=None)
    parser.add_argument("--crash-during", choices=["tool_call"], default=None)
    args = parser.parse_args()

    if not args.resume:
        for marker in (DECISION_SEALED_MARKER, TOOL_STARTED_MARKER):
            if os.path.exists(marker):
                os.remove(marker)

    init_backend(app_name="kill-mid-deploy")
    node_log = NodeLog(DB_PATH)

    def deploy_wrapper(version: str) -> dict:
        return deploy_server(version, crash_during=(args.crash_during == "tool_call"))

    tool_registry = {
        "deploy_server": Tool(name="deploy_server", fn=deploy_wrapper, effect_class=EffectClass.NON_IDEMPOTENT_WRITE),
    }

    def seal_marker_hook():
        if args.crash_after == "decision_sealed":
            with open(DECISION_SEALED_MARKER, "w") as f:
                f.write("sealed")

    run_step = make_run_step(node_log, TENANT_ID, TRAJECTORY_ID, tool_registry, on_decision_sealed=seal_marker_hook)

    try:
        with SetWorkflowID(TRAJECTORY_ID):
            results = run_step(step_n=1, model_call=model_call, context={})
    except BlockedNeedsGate as e:
        print(f"BLOCKED_NEEDS_GATE: {e}")
        sys.exit(0)

    if args.resume:
        print("Resumed. deploy_server executed exactly once.")
    print(f"results={results}")


if __name__ == "__main__":
    main()
```

- [ ] **Step 5: Write `test/e2e/_util.py`**

```python
# test/e2e/_util.py
import os
import subprocess
import time


def hard_kill(proc: subprocess.Popen) -> None:
    """Abruptly terminate a process to simulate a real crash. Popen.kill()
    already maps to SIGKILL on POSIX and TerminateProcess on Windows -- both
    are non-graceful hard kills, so no platform branching is needed."""
    proc.kill()
    proc.wait()


def wait_for_marker(path: str, timeout: float = 10.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        if os.path.exists(path):
            return
        time.sleep(0.05)
    raise TimeoutError(f"marker file {path!r} did not appear within {timeout}s")


def read_counter(path: str) -> int:
    if not os.path.exists(path):
        return 0
    return int(open(path).read().strip() or "0")


def cleanup(*paths: str) -> None:
    for p in paths:
        if os.path.exists(p):
            os.remove(p)
```

- [ ] **Step 6: Write the three failing crash-scenario tests**

```python
# test/e2e/test_seal_resume_crash.py
import subprocess

from test.e2e._util import cleanup, hard_kill, read_counter, wait_for_marker

AGENT = ["python", "examples/kill-mid-deploy/agent.py"]
ARTIFACTS = (
    "test_model_call_count.txt", "test_deploy_side_effect_count.txt",
    "decision_sealed.marker", "tool_started.marker", "kill_mid_deploy.sqlite",
)


def test_crash_before_seal_allows_reinference():
    """Scenario 1: crash before DECISION is sealed -> resume -> new
    inference is allowed (cheap, no side effect has happened yet)."""
    cleanup(*ARTIFACTS)

    proc = subprocess.Popen(AGENT)
    hard_kill(proc)  # kill immediately, before the seal marker can appear

    subprocess.run(AGENT + ["--resume"], check=True)

    assert read_counter("test_model_call_count.txt") >= 1


def test_crash_after_seal_before_tool_completes_no_reinfer():
    """Scenario 2: crash after DECISION seal but before tool execution
    completes -> resume -> tool executes to seal, model inference is NOT
    called a second time (assert the counter, not just 'tools ran once')."""
    cleanup(*ARTIFACTS)

    proc = subprocess.Popen(AGENT + ["--crash-after=decision_sealed"])
    wait_for_marker("decision_sealed.marker")
    hard_kill(proc)

    subprocess.run(AGENT + ["--resume"], check=True)

    assert read_counter("test_model_call_count.txt") == 1, "model was invoked more than once across resume"


def test_crash_mid_nonidempotent_tool_blocks_not_retries():
    """Scenario 3: crash mid non-idempotent tool -> resume -> call is
    BLOCKED_NEEDS_GATE, nothing auto-retries."""
    cleanup(*ARTIFACTS)

    proc = subprocess.Popen(AGENT + ["--crash-during=tool_call"])
    wait_for_marker("tool_started.marker")
    hard_kill(proc)

    result = subprocess.run(AGENT + ["--resume"], capture_output=True, text=True)

    assert read_counter("test_deploy_side_effect_count.txt") <= 1
    assert "BLOCKED_NEEDS_GATE" in result.stdout + result.stderr
```

- [ ] **Step 7: Run tests to verify they fail**

```bash
pytest test/e2e/test_seal_resume_crash.py -v
```

Expected: FAIL (module/agent not wired yet, or assertions fail because gate/adapter behavior isn't right).

- [ ] **Step 8: Run tests to verify they pass**

```bash
pytest test/e2e/test_seal_resume_crash.py -v
```

Expected: 3 passed. This is the step most likely to eat unplanned debugging time (sentinel-file timing flakiness) — that's normal for this test shape per the milestone issue, not a sign something's wrong. Tune the `time.sleep` windows in `agent.py`/`_util.py` if kills land inconsistently.

- [ ] **Step 9: Commit**

```bash
git add pkg/trajectory_ir/resume/gate.py
git commit -s -m "feat: add block-and-gate for non-idempotent tool crash recovery"
git add pkg/trajectory_ir/resume/step.py
git commit -s -m "feat: add durable run_step workflow wiring inference, seal, and gated tools"
git add examples/kill-mid-deploy/agent.py
git commit -s -m "feat: add kill-mid-deploy fixture agent shared by e2e, conformance, and demo"
git add test/e2e/_util.py test/e2e/test_seal_resume_crash.py
git commit -s -m "test: verify all three seal/resume crash scenarios via real SIGKILL"
```

---

### Task 8: Client SDK

**Files:**
- Create: `client/python/trajectory_client.py`
- Test: `test/unit/test_client_sdk.py`

**Interfaces:**
- Consumes: `NodeLog` (Task 4), `Tool`/`EffectClass` (Task 5), `init_backend` (Task 6), `make_gated_tool_call` (Task 7).
- Produces: `open_trajectory`, `project`, `seal_decision`, `exec_tool`, `commit_step`, `resume` — the 6 calls, no more.

- [ ] **Step 1: Write the failing tests**

```python
# test/unit/test_client_sdk.py
import os
import tempfile

import pytest

from client.python.trajectory_client import (
    open_trajectory, project, seal_decision, exec_tool, commit_step,
)
from trajectory_ir.runtime.tool import Tool
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.log import NodeLog


@pytest.fixture
def db_path(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    return str(tmp_path / "test_client.sqlite")


def test_project_appends_project_context_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="t1", db_path=db_path)
    project(traj, step_n=1, context={"foo": "bar"})
    assert NodeLog(db_path).has("t1", 1, "PROJECT_CONTEXT")


def test_seal_decision_appends_decision_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="t1", db_path=db_path)
    seal_decision(traj, step_n=1, plan={"tool_calls": []})
    assert NodeLog(db_path).has("t1", 1, "DECISION")


def test_exec_tool_runs_idempotent_write_directly(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="t1", db_path=db_path)
    tool = Tool(name="noop", fn=lambda x: x + 1, effect_class=EffectClass.IDEMPOTENT_WRITE)
    result = exec_tool(traj, step_n=1, call={"args": {"x": 1}}, tool=tool)
    assert result.result == 2


def test_commit_step_appends_commit_step_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="t1", db_path=db_path)
    commit_step(traj, step_n=1)
    assert NodeLog(db_path).has("t1", 1, "COMMIT_STEP")
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
pytest test/unit/test_client_sdk.py -v
```

Expected: FAIL with `ModuleNotFoundError: No module named 'client.python.trajectory_client'`.

- [ ] **Step 3: Implement**

```python
# client/python/trajectory_client.py
from dataclasses import dataclass
from typing import Any

from drivers.durable_backend.dbos.adapter import init_backend
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import make_gated_tool_call
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool


@dataclass
class Trajectory:
    trajectory_id: str
    tenant_id: str
    db_path: str


@dataclass
class ProjectContext:
    step_n: int
    context: dict


@dataclass
class Decision:
    step_n: int
    plan: dict


@dataclass
class ToolResult:
    step_n: int
    result: Any


def open_trajectory(tenant_id: str, trajectory_id: str, db_path: str = "trajectory.sqlite") -> Trajectory:
    init_backend(app_name=trajectory_id)
    return Trajectory(trajectory_id=trajectory_id, tenant_id=tenant_id, db_path=db_path)


def project(trajectory: Trajectory, step_n: int, context: dict) -> ProjectContext:
    NodeLog(trajectory.db_path).append(
        "PROJECT_CONTEXT", step_n, context, trajectory.trajectory_id, trajectory.tenant_id, seq=0
    )
    return ProjectContext(step_n=step_n, context=context)


def seal_decision(trajectory: Trajectory, step_n: int, plan: dict) -> Decision:
    NodeLog(trajectory.db_path).append(
        "DECISION", step_n, {"plan": plan}, trajectory.trajectory_id, trajectory.tenant_id, seq=1
    )
    return Decision(step_n=step_n, plan=plan)


def exec_tool(trajectory: Trajectory, step_n: int, call: dict, tool: Tool) -> ToolResult:
    log = NodeLog(trajectory.db_path)
    if tool.effect_class == EffectClass.NON_IDEMPOTENT_WRITE:
        fn = make_gated_tool_call(
            log, trajectory.trajectory_id, trajectory.tenant_id, step_n, seq=2,
            tool_name=tool.name, tool_fn=tool.fn,
        )
    else:
        fn = tool.fn
    result = fn(**call["args"])
    return ToolResult(step_n=step_n, result=result)


def commit_step(trajectory: Trajectory, step_n: int) -> None:
    NodeLog(trajectory.db_path).append(
        "COMMIT_STEP", step_n, {}, trajectory.trajectory_id, trajectory.tenant_id, seq=99
    )


def resume(trajectory_id: str, tenant_id: str = "demo", db_path: str = "trajectory.sqlite") -> Trajectory:
    init_backend(app_name=trajectory_id)
    return Trajectory(trajectory_id=trajectory_id, tenant_id=tenant_id, db_path=db_path)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
pytest test/unit/test_client_sdk.py -v
```

Expected: 4 passed.

- [ ] **Step 5: Commit**

```bash
git add client/python/trajectory_client.py
git commit -s -m "feat: add 6-call client SDK surface over the durable core"
git add test/unit/test_client_sdk.py
git commit -s -m "test: verify client SDK calls append expected node kinds"
```

---

### Task 9: R01 conformance test

**Files:**
- Create: `conformance/r01_seal_resume_test.py`

**Interfaces:**
- Consumes: `examples/kill-mid-deploy/agent.py` (Task 7), `test/e2e/_util.py` helpers (Task 7) — imported directly since both are already-built shared fixtures; no new production code this task.

- [ ] **Step 1: Write the test**

```python
# conformance/r01_seal_resume_test.py
import subprocess

from test.e2e._util import cleanup, hard_kill, read_counter, wait_for_marker

AGENT = ["python", "examples/kill-mid-deploy/agent.py"]
ARTIFACTS = (
    "test_model_call_count.txt", "test_deploy_side_effect_count.txt",
    "decision_sealed.marker", "tool_started.marker", "kill_mid_deploy.sqlite",
)


def test_r01_no_reinfer_after_seal():
    cleanup(*ARTIFACTS)

    proc = subprocess.Popen(AGENT + ["--crash-after=decision_sealed"])
    wait_for_marker("decision_sealed.marker")
    hard_kill(proc)

    subprocess.run(AGENT + ["--resume"], check=True)

    assert read_counter("test_model_call_count.txt") == 1, "model was invoked more than once across resume"
```

- [ ] **Step 2: Run and verify it passes**

```bash
pytest conformance/r01_seal_resume_test.py -v
```

Expected: 1 passed. This is functionally the same assertion as `test/e2e/test_seal_resume_crash.py::test_crash_after_seal_before_tool_completes_no_reinfer` — it exists separately because the milestone's Definition of Done names this exact file/path.

- [ ] **Step 3: Commit**

```bash
git add conformance/r01_seal_resume_test.py
git commit -s -m "test: add R01 conformance test for no-reinference-after-seal"
```

---

### Task 10: R02 conformance test

**Files:**
- Create: `conformance/r02_non_idempotent_test.py`

**Interfaces:**
- Consumes: same shared fixtures as Task 9.

- [ ] **Step 1: Write the test**

```python
# conformance/r02_non_idempotent_test.py
import subprocess

from test.e2e._util import cleanup, hard_kill, read_counter, wait_for_marker

AGENT = ["python", "examples/kill-mid-deploy/agent.py"]
ARTIFACTS = (
    "test_model_call_count.txt", "test_deploy_side_effect_count.txt",
    "decision_sealed.marker", "tool_started.marker", "kill_mid_deploy.sqlite",
)


def test_r02_crash_mid_tool_blocks_not_retries():
    cleanup(*ARTIFACTS)

    proc = subprocess.Popen(AGENT + ["--crash-during=tool_call"])
    wait_for_marker("tool_started.marker")
    hard_kill(proc)

    result = subprocess.run(AGENT + ["--resume"], capture_output=True, text=True)

    assert read_counter("test_deploy_side_effect_count.txt") <= 1, "deploy_server side effect ran more than once"
    assert "BLOCKED_NEEDS_GATE" in result.stdout + result.stderr, "resume did not report BLOCKED_NEEDS_GATE"
```

- [ ] **Step 2: Run and verify it passes**

```bash
pytest conformance/r02_non_idempotent_test.py -v
```

Expected: 1 passed.

- [ ] **Step 3: Run the full conformance suite together**

```bash
pytest conformance/ -v
```

Expected: 2 passed — this is Definition-of-Done item 2, verbatim.

- [ ] **Step 4: Commit**

```bash
git add conformance/r02_non_idempotent_test.py
git commit -s -m "test: add R02 conformance test for non-idempotent crash blocking"
```

---

### Task 11: kill-mid-deploy demo

**Files:**
- Create: `examples/kill-mid-deploy/run_demo.py`
- Create: `examples/kill-mid-deploy/README.md`

**Interfaces:**
- Consumes: `examples/kill-mid-deploy/agent.py` (Task 7) — no changes to it.

- [ ] **Step 1: Write `run_demo.py`**

```python
# examples/kill-mid-deploy/run_demo.py
"""Run (or resume) the kill-mid-deploy demo trajectory.

    python examples/kill-mid-deploy/run_demo.py
    # in another terminal, once you see "TOOL_CALL: deploy_server started":
    kill -9 <pid>
    python examples/kill-mid-deploy/run_demo.py --resume
    # expected output: "Resumed. deploy_server executed exactly once."
"""
import subprocess
import sys


def main():
    args = ["python", "examples/kill-mid-deploy/agent.py"]
    if "--resume" in sys.argv:
        args.append("--resume")
    subprocess.run(args, check=True)


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Write `README.md`**

```markdown
# kill-mid-deploy demo

Demonstrates the milestone's core claim: a crash mid-tool-execution never
silently re-runs the model or the side effect on resume.

## Run it

​```bash
python examples/kill-mid-deploy/run_demo.py
# in another terminal, once you see "TOOL_CALL: deploy_server started":
kill -9 <pid>
python examples/kill-mid-deploy/run_demo.py --resume
​```

Expected final output: `Resumed. deploy_server executed exactly once.`

## Recording

Once the demo runs reliably (no timing flakiness), record it:

​```bash
asciinema rec demo.cast
​```

A recorded run is the actual launch asset for this milestone — more
persuasive to anyone evaluating the project cold than the spec document
alone.
```

- [ ] **Step 3: Manually verify the demo end-to-end**

Run the exact sequence from the README in a real terminal (two terminal windows, real `kill -9`, not the pytest harness) and confirm the expected output appears.

- [ ] **Step 4: Commit**

```bash
git add examples/kill-mid-deploy/run_demo.py examples/kill-mid-deploy/README.md
git commit -s -m "docs: add kill-mid-deploy demo runner and recording instructions"
```

---

### Task 12: CI workflow and final clean-clone verification

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- None — this task wires up automation over everything built in Tasks 1-11.

- [ ] **Step 1: Write the CI workflow**

```yaml
# .github/workflows/ci.yml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: "3.11"
      - run: pip install -e ".[dev]"
      - run: pytest test/unit -v
      - run: pytest test/e2e -v
      - run: pytest conformance/ -v
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -s -m "build: add CI workflow running unit, e2e, and conformance tests"
git push
```

- [ ] **Step 3: Watch CI go green**

Confirm the pushed commit's CI run passes on GitHub before proceeding.

- [ ] **Step 4: Final clean-clone verification (Definition of Done, all 4 items)**

Run this exact sequence on a fresh clone — not the existing dev environment, since "works on my machine" is precisely the failure mode this project can't afford:

```bash
git clone <repo-url> fresh-clone && cd fresh-clone
python3 -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
pytest conformance/ -v
python examples/kill-mid-deploy/run_demo.py &
sleep 2 && kill -9 %1
python examples/kill-mid-deploy/run_demo.py --resume
```

Milestone v1.0.1 is done only when all four Definition-of-Done items pass on this fresh clone, DCO-signed commits are in place, and CI is green — not when it's merely documented as done.

---

## Self-Review Notes

- **Spec coverage:** every component in the design doc's Components table maps to a task (nodes→3, log→4, effects→5, adapter→6, resume/gate→7, client SDK→8, R01→9, R02→10, demo→11, CI→12). Both design-doc "Out of Scope" items (Postgres/S3, 7th SDK call) are correctly absent from any task.
- **Placeholder scan:** no TBD/TODO; the two "confirm against current docs" steps (Task 6 Step 1, Task 7 Step 3) are concrete verification actions with named URLs, not deferred work.
- **Type consistency:** `Tool(name, fn, effect_class)` defined once in Task 5, reused unmodified by Task 7 (gate/step), Task 8 (client SDK), and Task 7's fixture agent. `NodeLog.append(kind, step_n, payload, trajectory_id, tenant_id, seq)` signature is consistent everywhere it's called across Tasks 4, 7, 8. `make_run_step`'s `on_decision_sealed` hook (introduced in Task 7) is the only extension point added after the design doc was written — needed so the crash-injection fixture can mark "seal happened" without duplicating `run_step`'s internals.
