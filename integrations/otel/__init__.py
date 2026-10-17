"""Optional OpenTelemetry correlation helpers (export only)."""

from integrations.otel.correlate import (
    attach_seal_attributes,
    correlate_enabled,
    current_span,
    label_after_seal,
)

__all__ = [
    "attach_seal_attributes",
    "correlate_enabled",
    "current_span",
    "label_after_seal",
]
