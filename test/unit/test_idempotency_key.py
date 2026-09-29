"""Spec §7.3: seal-derived idempotency keys on TOOL_CALL nodes.

The key is for the *host* to forward to a remote API (Stripe-Idempotency-Key,
cloud APIs, DB unique constraints). Recording it in the IR log is not
exactly-once. Client-side block-and-gate is at-most-one automatic attempt.
"""

from client.python.trajectory_client import exec_tool, open_trajectory
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import make_gated_tool_call, make_plain_tool_call
from trajectory_ir.resume.idempotency import idempotency_key
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool


def test_idempotency_key_format_is_traj_step_seq():
    assert idempotency_key("traj-1", 1, 2) == "traj-1:1:2"


def test_idempotency_key_is_stable_for_same_sealed_slot():
    assert idempotency_key("t", 3, 4) == idempotency_key("t", 3, 4)


def test_idempotency_key_changes_when_slot_changes():
    assert idempotency_key("t", 1, 2) != idempotency_key("t", 1, 4)
    assert idempotency_key("t", 1, 2) != idempotency_key("t", 2, 2)
    assert idempotency_key("a", 1, 2) != idempotency_key("b", 1, 2)


def test_gated_tool_call_records_key_and_does_not_inject_into_tool_args(tmp_path):
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    seen_kwargs = {}

    def deploy(**kwargs):
        seen_kwargs.update(kwargs)
        return {"ok": True}

    gated = make_gated_tool_call(log, "t1", "demo", 1, 2, "deploy_server", deploy)
    gated(version="1.0.0")

    assert seen_kwargs == {"version": "1.0.0"}
    payload = _tool_call_payload(log, "t1", "demo", seq=2)
    assert payload["tool"] == "deploy_server"
    assert payload["args"] == {"version": "1.0.0"}
    assert payload["idempotency_key"] == "t1:1:2"


def test_plain_tool_call_records_the_same_key_shape(tmp_path):
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    plain = make_plain_tool_call(log, "t1", "demo", 1, 2, "echo", lambda msg: msg)
    plain(msg="hi")
    payload = _tool_call_payload(log, "t1", "demo", seq=2)
    assert payload["idempotency_key"] == idempotency_key("t1", 1, 2)
    assert payload["args"] == {"msg": "hi"}


def test_client_exec_tool_records_key(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db_path = str(tmp_path / "client.sqlite")
    traj = open_trajectory(tenant_id="demo", trajectory_id="client-1", db_path=db_path)
    tool = Tool(name="echo", fn=lambda x: x, effect_class=EffectClass.PURE)
    exec_tool(traj, step_n=1, call={"args": {"x": 7}}, tool=tool, seq=2)
    payload = _tool_call_payload(NodeLog(db_path), "client-1", "demo", seq=2)
    assert payload["idempotency_key"] == "client-1:1:2"


def _tool_call_payload(log: NodeLog, trajectory_id: str, tenant_id: str, seq: int) -> dict:
    for row in log.list_nodes(trajectory_id, tenant_id=tenant_id):
        if row["kind"] == "TOOL_CALL" and row["seq"] == seq:
            return row["payload"]
    raise AssertionError(f"no TOOL_CALL at seq={seq}")
