"""Attach sealed TrajIR facts onto an existing OTel span.

This is correlation and export only. It runs after an in-process seal
(see integrations/langgraph/sidecar.py). A collector that sees
execute_tool after the span ends cannot seal before the world change.
That path is a logger, not Chain-of-Evidence. Do not use this module as a
substitute for TrajIRToolGuard.
"""

from __future__ import annotations

import contextvars
import os
from typing import Any

_SPAN: contextvars.ContextVar[Any] = contextvars.ContextVar("trajir_otel_span", default=None)


def push_span(span: Any) -> contextvars.Token:
    """Tests and hosts can pin the span that should receive the next label."""
    return _SPAN.set(span)


def reset_span(token: contextvars.Token) -> None:
    _SPAN.reset(token)


def current_span() -> Any:
    """Return the pinned span, or the active OpenTelemetry span when installed."""
    pinned = _SPAN.get()
    if pinned is not None:
        return pinned
    try:
        from opentelemetry import trace  # type: ignore[import-not-found]
    except ImportError:
        return None
    try:
        return trace.get_current_span()
    except Exception:
        return None


def correlate_enabled() -> bool:
    return os.environ.get("TRAJIR_OTEL_CORRELATE", "").strip().lower() in {
        "1",
        "true",
        "yes",
        "on",
    }


def attach_seal_attributes(
    span: Any,
    *,
    trajectory_id: str,
    tenant_id: str,
    step_n: int,
    node_id: str = "",
    content_hash: str = "",
    effect_class: str = "",
    tool_name: str = "",
    idempotency_key: str = "",
) -> None:
    """Set TrajIR attributes on ``span`` when correlation is enabled.

    ``span`` is duck-typed (OpenTelemetry Span). Missing set_attribute is a
    no-op so tests can pass a simple namespace object.
    """
    if span is None or not correlate_enabled():
        return
    setter = getattr(span, "set_attribute", None)
    if not callable(setter):
        return
    attrs = {
        "trajir.trajectory_id": trajectory_id,
        "trajir.tenant_id": tenant_id,
        "trajir.step_n": step_n,
        "trajir.control": "in_process_wrap",
    }
    if node_id:
        attrs["trajir.node_id"] = node_id
    if content_hash:
        attrs["trajir.content_hash"] = content_hash
    if effect_class:
        attrs["trajir.effect_class"] = effect_class
    if tool_name:
        attrs["trajir.tool_name"] = tool_name
    if idempotency_key:
        attrs["trajir.idempotency_key"] = idempotency_key
    for key, value in attrs.items():
        setter(key, value)


def label_after_seal(
    span: Any,
    *,
    decision_node_id: str,
    trajectory_id: str,
    tenant_id: str,
    step_n: int,
    content_hash: str = "",
    effect_class: str = "",
    tool_name: str = "",
    idempotency_key: str = "",
) -> None:
    """Label ``span`` only when an in-process seal already produced a decision id.

    An empty decision id means the seal has not happened. Nothing is copied.
    """
    if not decision_node_id:
        return
    attach_seal_attributes(
        span,
        trajectory_id=trajectory_id,
        tenant_id=tenant_id,
        step_n=step_n,
        node_id=decision_node_id,
        content_hash=content_hash,
        effect_class=effect_class,
        tool_name=tool_name,
        idempotency_key=idempotency_key,
    )
