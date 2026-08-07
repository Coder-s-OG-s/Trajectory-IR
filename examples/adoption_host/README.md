# Adoption host demo

Host owned seal loop that a newcomer can run after an editable install. Uses
**only public Python APIs**: client open / project / seal / exec / commit, plus
optional filesystem CAS and thin `.tir` export.

This is not an agent framework. The model is a stub function. Swap it for a
real LLM client without changing seal or tool execution.

## What it proves

1. `open_trajectory` → `project` → stub model → `seal_decision`
2. `exec_tool` for a **PURE** tool (`build_manifest`) and a **gated**
   `NON_IDEMPOTENT_WRITE` tool (`ship_release`)
3. `commit_step` finalizes the step
4. Optional `--with-package`: `put_artifact` into `FileSystemCAS`, thin
   `export_tir(..., cas=...)`, `load_tir` + `rehydrate_artifacts` with a
   byte match check
5. Optional `--sandbox`: non idempotent tools are rejected before the body runs

## How this differs from other examples

| Example | Intent |
|---------|--------|
| `examples/adoption_host/` (this) | Adoption: full host loop + optional CAS / thin package |
| `examples/host_loop/` | Minimal public client step (no packaging) |
| `examples/kill-mid-deploy/` | Crash safety / durable workflow resume |

## Run

From the repository root:

```bash
pip install -e ".[dev]"
python examples/adoption_host/run_demo.py
```

Expected exit code `0` and a short summary of tool results and node kinds.

Sandbox (should refuse `ship_release`):

```bash
python examples/adoption_host/run_demo.py --sandbox
```

Live step plus thin package + CAS rehydrate:

```bash
python examples/adoption_host/run_demo.py --with-package
```

Persist artifacts somewhere stable:

```bash
python examples/adoption_host/run_demo.py \
  --db ./adoption.sqlite \
  --with-package \
  --cas-root ./cas_root \
  --tir-out ./adoption-run.tir
```

## Design notes

1. Tool arguments in the sealed plan are **concrete known values** only
   (linear known args rule from the master README).
2. Seq allocation is `seq = 2 + 2*i` for the i-th tool call so `TOOL_CALL` /
   `TOOL_RESULT` slots do not overlap.
3. The host owns the artifact payload. CAS stores bytes; the IR log stores
   content addressed nodes. Thin packages carry hashes, not embedded blobs.
4. CI covers this example via `test/unit/test_adoption_host_example.py`.
