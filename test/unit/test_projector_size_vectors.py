"""Cross-language goldens for projector size units (RFC 8785 / JCS)."""

from __future__ import annotations

import json
from pathlib import Path

from trajectory_ir.runtime.projector import SIZE_METRIC, node_size_units

_ROOT = Path(__file__).resolve().parents[2]
_VECTORS = _ROOT / "testdata" / "projector_size_vectors.json"


def test_projector_size_vectors_match_python() -> None:
    data = json.loads(_VECTORS.read_text(encoding="utf-8"))
    assert data.get("version") == 1
    assert data.get("metric") == SIZE_METRIC
    assert data.get("cases"), "empty size vectors"
    for case in data["cases"]:
        got = node_size_units(case["node"])
        assert got == case["size_units"], case["name"]


def test_key_order_does_not_change_size_units() -> None:
    a = {
        "id": "x",
        "kind": "STATE_SET",
        "payload": {"a": 1, "b": 2},
        "step_n": 1,
        "seq": 0,
    }
    b = {
        "id": "x",
        "kind": "STATE_SET",
        "payload": {"b": 2, "a": 1},
        "step_n": 1,
        "seq": 0,
    }
    assert node_size_units(a) == node_size_units(b)
