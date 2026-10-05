from __future__ import annotations

import os

from integrations.otel.correlate import attach_seal_attributes, correlate_enabled


class _FakeSpan:
    def __init__(self) -> None:
        self.attrs: dict[str, object] = {}

    def set_attribute(self, key: str, value: object) -> None:
        self.attrs[key] = value


def test_correlate_disabled_by_default(monkeypatch) -> None:
    monkeypatch.delenv("TRAJIR_OTEL_CORRELATE", raising=False)
    assert correlate_enabled() is False
    span = _FakeSpan()
    attach_seal_attributes(span, trajectory_id="t", tenant_id="d", step_n=1, node_id="n")
    assert span.attrs == {}


def test_correlate_sets_attrs(monkeypatch) -> None:
    monkeypatch.setenv("TRAJIR_OTEL_CORRELATE", "1")
    span = _FakeSpan()
    attach_seal_attributes(
        span,
        trajectory_id="t1",
        tenant_id="demo",
        step_n=2,
        node_id="abc",
        effect_class="NON_IDEMPOTENT_WRITE",
        tool_name="ship_release",
        idempotency_key="deadbeef",
    )
    assert span.attrs["trajir.trajectory_id"] == "t1"
    assert span.attrs["trajir.control"] == "in_process_wrap"
    assert span.attrs["trajir.idempotency_key"] == "deadbeef"
    assert os.environ.get("TRAJIR_OTEL_CORRELATE") == "1"
