# Agent host loop example

Minimal **host** that owns the model call, tools, and trajectory step using only
the public Python client API. This is not an agent framework: it shows how a
host process should call Trajectory IR.

## What it demonstrates

1. `open_trajectory`
2. Stub model (no paid API; injectable function)
3. `project` then `seal_decision` with concrete known args only
4. `exec_tool` then `commit_step`
5. Optional sandbox mode (`mode=sandbox`) so non idempotent tools are rejected

## Run

From the repository root (editable install recommended):

```bash
pip install -e ".[dev]"
python examples/host_loop/run_host.py
```

Expected exit code `0` and a short summary of the step.

Sandbox demo (should refuse `deploy_server`):

```bash
python examples/host_loop/run_host.py --sandbox
```

## Design notes

1. The model is a pure function that returns a plan dict. Swap it for a real
   LLM client without changing seal or tool execution.
2. Tool arguments in the sealed plan are concrete values only (linear known
   args rule from the master README).
3. Crash safe durable workflow demos remain under `examples/kill-mid-deploy/`.
