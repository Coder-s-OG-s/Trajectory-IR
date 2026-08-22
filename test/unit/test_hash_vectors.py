"""Cross language golden vectors shared with the Go IR core (testdata/hash_vectors.json)."""

from __future__ import annotations

import json
from pathlib import Path

from trajectory_ir.runtime.nodes import Node, node_id, payload_hash

ROOT = Path(__file__).resolve().parents[2]
VECTORS = ROOT / "testdata" / "hash_vectors.json"


def _load_cases() -> list[dict]:
    data = json.loads(VECTORS.read_text(encoding="utf-8"))
    assert data.get("version") == 1
    return data["cases"]  # type: ignore[no-any-return]


def test_python_matches_committed_hash_vectors() -> None:
    for case in _load_cases():
        ph = payload_hash(case["payload"])
        assert ph == case["payload_hash"], case["name"]
        nid = node_id(
            case["tenant_id"],
            case["trajectory_id"],
            case["step_n"],
            case["seq"],
            case["kind"],
            ph,
        )
        assert nid == case["node_id"], case["name"]
        node = Node(
            kind=case["kind"],
            trajectory_id=case["trajectory_id"],
            tenant_id=case["tenant_id"],
            step_n=case["step_n"],
            seq=case["seq"],
            payload=case["payload"],
        )
        assert node.phash == case["payload_hash"], case["name"]
        assert node.id == case["node_id"], case["name"]


def test_key_order_pair_shares_identity() -> None:
    cases = {c["name"]: c for c in _load_cases()}
    a = cases["key_order_independent"]
    b = cases["key_order_reversed_same_as_above"]
    assert a["payload_hash"] == b["payload_hash"]
    assert a["node_id"] == b["node_id"]
