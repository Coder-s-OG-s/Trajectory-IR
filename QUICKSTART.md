# Quickstart: Trajectory IR

Get a **local** Trajectory IR checkout running. This matches APIs that exist on `main` today (Phase 1A library surface).

Trajectory IR is a **semantic layer** for agent runs: content-addressed nodes, effect classes, block-and-gate for non-idempotent tools, portable `.tir` packages, and optional sandbox mode. Crash detection and step memoization are delegated to a durable backend (DBOS for Python Phase 1A).

## Prerequisites

- Python **3.11+**
- Git
- Optional: Go **1.25.x** (only if you exercise `go/trajir`)

---

## 1. Install from a clone

```bash
git clone https://github.com/Coder-s-OG-s/Trajectory-IR.git
cd Trajectory-IR
python -m venv .venv
# Windows: .\.venv\Scripts\activate
# Unix:    source .venv/bin/activate
pip install -U pip
pip install -e ".[dev]"
```

PyPI publish of `trajectory-ir` may follow the `v0.1.0` tag; until then, **editable install from git** is the supported path.

---

## 2. Minimal Python flow (client + NodeLog)

```python
from client.python.trajectory_client import (
    open_trajectory,
    project,
    seal_decision,
    exec_tool,
    commit_step,
)
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.tool import Tool

def echo(msg: str) -> str:
    return msg

traj = open_trajectory(tenant_id="demo", trajectory_id="qs-1", db_path="qs.sqlite")
# Optional sandbox (R06): open_trajectory(..., mode="sandbox")  # rejects NON_IDEMPOTENT_WRITE

project(traj, step_n=1, context={"goal": "hello"})
seal_decision(traj, step_n=1, plan={"tool_calls": [{"name": "echo", "args": {"msg": "hi"}}]})

tool = Tool(name="echo", fn=echo, effect_class=EffectClass.PURE)
result = exec_tool(traj, step_n=1, call={"args": {"msg": "hi"}}, tool=tool, seq=2)
print(result.result)  # hi

commit_step(traj, step_n=1, seq=4)
```

### Durable step runner (DBOS)

For full seal/resume with DBOS steps, use `make_run_step` as in
`examples/kill-mid-deploy/agent.py` (used by R01/R02 and e2e tests).

```bash
# From repo root, isolate artifacts in a temp directory:
# see examples/kill-mid-deploy/README.md
pytest test/e2e conformance/r01_seal_resume_test.py conformance/r02_non_idempotent_test.py -q
```

---

## 3. Export / import a `.tir` package (R05)

```python
from trajectory_ir.package import export_tir, import_tir
from trajectory_ir.runtime.log import NodeLog

src = NodeLog("run.sqlite")
# ... append nodes for trajectory_id="t1", tenant_id="demo" ...

export_tir(src, "t1", "run.tir", mode="thin", tenant_id="demo")

dst = NodeLog("imported.sqlite")
pkg = import_tir("run.tir", dst)  # verifies node ids; appends idempotently
print(pkg.manifest["node_count"], pkg.manifest["mode"])
```

- **Fat** mode: pass `artifacts=` and `artifact_bytes=` (see `test/unit/test_tir_package.py`).
- **Redacted** export: `export_tir(..., redacted=True)` (heuristic; review before sharing).
- **Import** always verifies; do not use unverified import into a durable log.

---

## 4. Projector + redaction (R04 / R08)

```python
from trajectory_ir.runtime.projector import project_context, BudgetImpossible
from trajectory_ir.runtime.redact import redact_projection_context

# nodes: list of node dicts (id, kind, payload, step_n, seq, ...)
try:
    result = project_context(nodes, budget=50_000, pinned_ids=set())
except BudgetImpossible as e:
    print(e)  # CONSTRAINT/pinned alone exceed budget — never silent drop
else:
    safe = redact_projection_context(result.context)
    # pass `safe` into client.project(...) or your model call
```

---

## 5. Sandbox mode (R06)

```python
from client.python.trajectory_client import open_trajectory, exec_tool
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.tool import Tool

traj = open_trajectory("demo", "sandbox-1", db_path="s.sqlite", mode="sandbox")
# NON_IDEMPOTENT_WRITE → SandboxForbidden before the tool body runs
# PURE / READ_ONLY / etc. still allowed
```

---

## 6. Graft artifact refs (R07)

```python
from trajectory_ir.runtime.graft import graft_artifact_ref
from trajectory_ir.runtime.log import NodeLog

# source_nodes from list_nodes(..., tenant_id=...); must include ARTIFACT_PUT/REF
# never copies THOUGHT nodes
graft_artifact_ref(
    target_log,
    content_hash="<64-hex>",
    target_trajectory_id="child",
    target_tenant_id="demo",
    seq=0,
    step_n=1,
    source_nodes=source_nodes,
)
```

---

## 7. Go (optional)

```bash
cd go
go test ./...
# See go/README.md for client, Temporal env vars, and .tir package APIs
```

---

## 8. Thin package + local CAS

```python
from trajectory_ir.package import export_tir, load_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.storage import FileSystemCAS, put_artifact, rehydrate_artifacts

store = FileSystemCAS("./cas_root")
log = NodeLog("traj.sqlite")
# ... append nodes as usual ...
ref = put_artifact(store, b"bytes from a tool", logical_path="out.bin")
export_tir(log, "my-traj", "run.tir", mode="thin", artifacts=[ref], tenant_id="demo", cas=store)
pkg = load_tir("run.tir")
rehydrated = rehydrate_artifacts(store, pkg.artifacts_manifest)
```

## What's next

| Doc | Purpose |
|-----|---------|
| [docs/PHASE_1A_STATUS.md](docs/PHASE_1A_STATUS.md) | What shipped vs deferred |
| [README.md](README.md) | Master specification |
| [CONTRIBUTING.md](CONTRIBUTING.md) | DCO, CI, local dev |
