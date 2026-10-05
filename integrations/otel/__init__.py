"""Optional OpenTelemetry correlation helpers (export only)."""

from integrations.otel.correlate import attach_seal_attributes, correlate_enabled

__all__ = ["attach_seal_attributes", "correlate_enabled"]
