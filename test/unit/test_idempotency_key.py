"""Spec §7.3: hashed seal-derived idempotency keys on TOOL_CALL nodes.

The key is for the *host* to forward to a remote API (Idempotency-Key,
Stripe, cloud APIs, DB unique constraints). Recording it in the IR log is not
exactly-once. Client-side block-and-gate is at-most-one automatic attempt.
The key is never injected into tool kwargs.
"""

import json
from pathlib import Path

from client.python.trajectory_client import exec_tool, open_trajectory
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import make_gated_tool_call, make_plain_tool_call
from trajectory_ir.resume.idempotency import (
    current_call_meta,
    current_idempotency_key,
    idempotency_key,
    idempotency_key_header,
)
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool

ROOT = Path(__file__).resolve().parents[2]
VECTORS = json.loads((ROOT / "testdata" / "idempotency_vectors.json").read_text(encoding="utf-8"))


def test_python_matches_committed_idempotency_vectors():
    for case in VECTORS["cases"]:
        got = idempotency_key(
            case["tenant_id"],
            case["trajectory_id"],
            case["step_n"],
            case["seq"],
        )
        assert got == case["idempotency_key"], case["name"]
        assert len(got) == 64
        assert got == got.lower()


def test_idempotency_key_is_stable_for_same_sealed_slot():
    assert idempotency_key("demo", "t", 3, 4) == idempotency_key("demo", "t", 3, 4)


def test_idempotency_key_nul_inside_id_does_not_alias():
    assert idempotency_key("a", "b\x00c", 1, 2) != idempotency_key("a\x00b", "c", 1, 2)


def test_idempotency_key_changes_when_slot_or_tenant_changes():
    base = idempotency_key("demo", "t", 1, 2)
    assert base != idempotency_key("demo", "t", 1, 4)
    assert base != idempotency_key("demo", "t", 2, 2)
    assert base != idempotency_key("demo", "b", 1, 2)
    assert base != idempotency_key("other", "t", 1, 2)


def test_idempotency_key_header_is_standard_name():
    key = idempotency_key("demo", "t1", 1, 2)
    assert idempotency_key_header(key) == {"Idempotency-Key": key}


def test_gated_tool_call_records_key_and_does_not_inject_into_tool_args(tmp_path):
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    seen_kwargs = {}
    seen_key = {}

    def deploy(**kwargs):
        seen_kwargs.update(kwargs)
        seen_key["key"] = current_idempotency_key()
        seen_key["meta"] = current_call_meta()
        return {"ok": True}

    gated = make_gated_tool_call(log, "t1", "demo", 1, 2, "deploy_server", deploy)
    gated(version="1.0.0")

    want = idempotency_key("demo", "t1", 1, 2)
    assert seen_kwargs == {"version": "1.0.0"}
    assert seen_key["key"] == want
    assert seen_key["meta"] is not None
    assert seen_key["meta"].idempotency_key == want
    assert seen_key["meta"].tenant_id == "demo"
    assert current_idempotency_key() is None

    payload = _tool_call_payload(log, "t1", "demo", seq=2)
    assert payload["tool"] == "deploy_server"
    assert payload["args"] == {"version": "1.0.0"}
    assert payload["idempotency_key"] == want
    assert "idempotency_key" not in payload["args"]


def test_plain_tool_call_records_the_same_key_shape(tmp_path):
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    plain = make_plain_tool_call(log, "t1", "demo", 1, 2, "echo", lambda msg: msg)
    plain(msg="hi")
    payload = _tool_call_payload(log, "t1", "demo", seq=2)
    assert payload["idempotency_key"] == idempotency_key("demo", "t1", 1, 2)
    assert payload["args"] == {"msg": "hi"}


def test_client_exec_tool_records_key(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db_path = str(tmp_path / "client.sqlite")
    traj = open_trajectory(tenant_id="demo", trajectory_id="client-1", db_path=db_path)
    tool = Tool(name="echo", fn=lambda x: x, effect_class=EffectClass.PURE)
    exec_tool(traj, step_n=1, call={"args": {"x": 7}}, tool=tool, seq=2)
    payload = _tool_call_payload(NodeLog(db_path), "client-1", "demo", seq=2)
    assert payload["idempotency_key"] == idempotency_key("demo", "client-1", 1, 2)


def _tool_call_payload(log: NodeLog, trajectory_id: str, tenant_id: str, seq: int) -> dict:
    for row in log.list_nodes(trajectory_id, tenant_id=tenant_id):
        if row["kind"] == "TOOL_CALL" and row["seq"] == seq:
            return row["payload"]
    raise AssertionError(f"no TOOL_CALL at seq={seq}")
