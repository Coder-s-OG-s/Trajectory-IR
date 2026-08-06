# Restate durable backend adapter

Optional second durable execution backend for Trajectory IR (issue #66).

## Why this exists

The master README names DBOS as the Phase 1A default and welcomes Restate
once the adapter interface is stable. This package provides:

1. The **same call surface** as `drivers.durable_backend.dbos.adapter`
   (`init_backend`, `durable_infer`, `durable_tool`, `durable_workflow`)
2. A **process local memo** implementation for development and unit tests
3. Injection into `make_run_step` without hardcoding Restate into the IR core

Production Restate SDK wiring is intentionally thin here: operators should use
Restate's own durable handlers for crash detection and lease/heartbeat. Trajectory
IR only needs step memoization for model and tool calls.

## Local memo (default in this package)

```python
from drivers.durable_backend.restate import (
    durable_infer,
    durable_tool,
    durable_workflow,
    init_backend,
    workflow_scope,
)
from trajectory_ir.resume.step import make_run_step

init_backend()
run_step = make_run_step(
    node_log,
    tenant_id,
    trajectory_id,
    tool_registry,
    durable_infer_fn=durable_infer,
    durable_tool_fn=durable_tool,
    durable_workflow_fn=durable_workflow,
)

with workflow_scope(trajectory_id):
    run_step(step_n=1, model_call=model, context={})
```

On a second call with the same workflow id, memoized infer/tool steps return
prior results without re-executing the function body (R01 spirit).

## Real Restate cluster (operator sketch)

1. Run Restate server (`restate-server` / Docker image from Restate docs).
2. Implement a Restate service whose handlers wrap your tool and model
   functions with Restate's durable promise / journal semantics.
3. Keep using `make_run_step` with injectable `durable_*` callables that
   invoke those handlers, **or** call Restate from a thin host and only use
   Trajectory IR for NodeLog seals and `.tir` export.
4. Never add custom crash loops in `pkg/resume`.

Environment variables (suggested for hosts that dial Restate):

| Variable | Purpose |
|----------|---------|
| `RESTATE_URL` | Ingress URL (example: `http://localhost:8080`) |
| `RESTATE_ADMIN_URL` | Admin API if you register services from tooling |

Secrets for managed Restate stay in the environment or a secret manager.

## What this package does not do

1. Ship a full multi service Restate application layout
2. Replace DBOS as the default for `make_run_step` (DBOS remains default)
3. Implement package signatures, Fluid, or CAS
