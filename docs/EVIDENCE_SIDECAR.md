# Evidence Sidecar

**Product:** a sealed evidence sidecar for agent tool calls. Companies keep their
host (LangGraph today). TrajIR seals the `DECISION` **before** the tool mutates
the world, classifies the effect, binds a hashed idempotency key, and emits a
portable `.tir` pack anyone can audit offline with `trajir verify`.

This is the TURNING_POINT install story: **wrap + verify**, not a host rewrite.

**Honesty lock**

- Empty adopters stay empty. Do not invent production users.
- Token meters are estimates (`ceil(chars/4)`), not invoices.
- Not exactly-once. Not a process sandbox. Not Sigstore (deferred).
- OTel correlation **labels** spans after an in-process seal. A post-hoc OTLP
  collector that sees `execute_tool` after the tool finished is a logger, not
  Chain-of-Evidence.

Local soak notes (gitignored): `docs/LOCAL_EVIDENCE_SIDECAR.md`.

---

## One-command install (local)

From the worktree root:

```powershell
pwsh -File scripts\install_evidence_sidecar.ps1
```

This builds `go/trajir.exe` (the verify CLI) and prints the next steps.

Optional Python editable install (repo already on `PYTHONPATH` for tests):

```powershell
pip install -e ".[langgraph]"   # only if you use LangGraph ToolNode
```

---

## Stub demo (no LangGraph required)

```powershell
python integrations/langgraph/demo_stub.py
go\trajir.exe verify .local-evidence\langgraph-demo.tir
```

Expect `OK` on a good pack. Flip a byte and verify again — expect `FAIL`.

---

## LangGraph ToolNode wrap

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
# ... run graph ...
path = guard.export_tir(".local-evidence/lg-live.tir")
guard.close()
```

Then:

```powershell
go\trajir.exe verify .local-evidence\lg-live.tir
# or JSON for console / CI
go\trajir.exe verify --json .local-evidence\lg-live.tir
```

---

## Operator console (single port **8787**)

Run **one** console process from this worktree:

```powershell
$env:TRAJIR_CONSOLE_DATA = "$PWD\.console-data"
cd go
go run ./cmd/trajir-console -addr 127.0.0.1:8787
```

Emit into it:

```powershell
$env:TRAJIR_CONSOLE_SINK = "http"
$env:TRAJIR_CONSOLE_URL = "http://127.0.0.1:8787"
python integrations/langgraph/demo_stub.py
```

Open [http://127.0.0.1:8787/?id=langgraph-demo](http://127.0.0.1:8787/?id=langgraph-demo)
and use the **Evidence** tab (seal-before-execute, tool keys, `trajir verify`).

Do not run a second console on another port.

Event vocabulary: [CONSOLE_EVENTS.md](CONSOLE_EVENTS.md) (`audit.completed`, TOOL_CALL fields).

---

## OTel correlate (export only)

```python
import os
os.environ["TRAJIR_OTEL_CORRELATE"] = "1"
from integrations.otel.correlate import attach_seal_attributes

attach_seal_attributes(span, trajectory_id=..., tenant_id=..., step_n=1, ...)
```

Seal first via `TrajIRToolGuard`. Then label the span. Details:
[integrations/otel/README.md](../integrations/otel/README.md).

---

## What verify checks

`trajir verify pack.tir` fails closed when:

- Package load / hash chain is broken
- A mutating tool ran without a prior `DECISION` on that step (seal-before-execute)
- An open-world primitive is tagged as a safer effect class (lie)
- `--require-signature` is set and the pack has no `SIGNATURE` member

Console Evidence can be **stricter** than verify on pure/read-only tools; the
UI labels that difference.

---

## Smoke / tests

```powershell
pwsh -File scripts\local_evidence_sidecar_smoke.ps1
python -m pytest integrations/langgraph/test_sidecar.py test/integration/test_evidence_sidecar.py -q
```

---

## Non-goals (until demand)

- ADK wrap (LangGraph is the reference host for this cut)
- Sigstore / cosign on packs
- Post-hoc OTLP seal claims
- Fake adopters, Harborline-as-production, multi-tenant SaaS

---

## Related

- [integrations/langgraph/README.md](../integrations/langgraph/README.md)
- [INTEGRATIONS.md](INTEGRATIONS.md) (MCP + Evidence)
- [CONSOLE_RUNBOOK.md](CONSOLE_RUNBOOK.md)
- [SCOPE_AND_NON_GOALS.md](SCOPE_AND_NON_GOALS.md)
