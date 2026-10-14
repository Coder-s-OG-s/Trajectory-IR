# OTel correlation (export only)

Canonical product guide: **[docs/EVIDENCE_SIDECAR.md](../../docs/EVIDENCE_SIDECAR.md)**.

Attach sealed TrajIR identifiers onto spans you already create.

## Rule

Seal happens in `integrations/langgraph` (or your own `execute_tool` wrap).
This package only **labels** the span after that seal.

An OTLP collector that receives `execute_tool` after the tool finished is a
logger. It cannot claim Chain-of-Evidence.

## Usage

```python
import os
os.environ["TRAJIR_OTEL_CORRELATE"] = "1"

from integrations.otel.correlate import attach_seal_attributes

attach_seal_attributes(
    span,
    trajectory_id="lg-1",
    tenant_id="demo",
    step_n=1,
    node_id=decision_id,
    effect_class="NON_IDEMPOTENT_WRITE",
    tool_name="ship_release",
    idempotency_key=key,
)
```

No OpenTelemetry SDK is required to import this module. Pass any object with
`set_attribute(key, value)`.
