# LangGraph evidence sidecar (local)

Copy-paste path for TURNING_POINT.md: wrap `execute_tool` so a `DECISION` is
sealed **before** the tool mutates the world, then export a `.tir` and run
`trajir verify`.

This folder is local work on branch `local/evidence-sidecar`. It is not pushed.

## Quick smoke (no LangGraph install required)

From the worktree root `H:\Trajectory-IR-wt-evidence`:

```powershell
python integrations/langgraph/demo_stub.py
go\trajir.exe verify .local-evidence\langgraph-demo.tir
```

## With LangGraph ToolNode

```powershell
pip install langgraph langchain-core
```

```python
from langgraph.prebuilt import ToolNode
from integrations.langgraph.sidecar import TrajIRToolGuard
from trajectory_ir.effects import EffectClass

guard = TrajIRToolGuard(
    tenant_id="demo",
    trajectory_id="lg-live",
    work_dir=".local-evidence/lg-live",
    effect_hints={"ship_release": EffectClass.NON_IDEMPOTENT_WRITE},
)
tools = ToolNode(my_tools, wrap_tool_call=guard.wrap_tool_call)
# ... run graph ...
guard.export_tir(".local-evidence/lg-live.tir")
guard.close()
```

Optional console:

```powershell
$env:TRAJIR_CONSOLE_SINK = "http"
$env:TRAJIR_CONSOLE_URL = "http://127.0.0.1:8787"
```

## What this proves

1. Seal before tool body
2. Open-world names refuse safer effect tags
3. Hashed idempotency key on `TOOL_CALL` (never in tool kwargs)
4. Thin `.tir` passes `trajir verify`
