# LangGraph evidence sidecar

Canonical install and honesty lock: **[docs/EVIDENCE_SIDECAR.md](../../docs/EVIDENCE_SIDECAR.md)**.

Wrap `execute_tool` so a `DECISION` is sealed **before** the tool mutates the
world, then export a `.tir` and run `trajir verify`.

## Quick smoke (no LangGraph install)

```powershell
pwsh -File scripts\install_evidence_sidecar.ps1
python integrations/langgraph/demo_stub.py
go\trajir.exe verify .local-evidence\langgraph-demo.tir
```

## With LangGraph ToolNode

```powershell
pip install -e ".[langgraph]"
```

```python
from langgraph.prebuilt import ToolNode
from integrations.langgraph import TrajIRToolGuard
from trajectory_ir.effects import EffectClass

guard = TrajIRToolGuard(
    tenant_id="demo",
    trajectory_id="lg-live",
    work_dir=".local-evidence/lg-live",
    effect_hints={"ship_release": EffectClass.NON_IDEMPOTENT_WRITE},
)
tools = ToolNode(my_tools, wrap_tool_call=guard.wrap_tool_call)
path = guard.export_tir(".local-evidence/lg-live.tir")
guard.close()
```

## Console (8787 only)

```powershell
$env:TRAJIR_CONSOLE_SINK = "http"
$env:TRAJIR_CONSOLE_URL = "http://127.0.0.1:8787"
python integrations/langgraph/demo_stub.py
# Evidence tab: http://127.0.0.1:8787/?id=langgraph-demo
```

`demo_stub` emits `audit.completed` after verify when the sink is set.

## Shared helpers

- `TrajIRToolGuard` — seal / classify / exec / export
- `run_verify` / `verify_and_note_audit` — `integrations.langgraph.verify_pack`

## What this proves

1. Seal before tool body
2. Open-world names refuse safer effect tags
3. Hashed idempotency key on `TOOL_CALL` (never in tool kwargs)
4. Thin `.tir` passes `trajir verify`
