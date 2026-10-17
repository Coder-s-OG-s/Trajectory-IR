import hashlib
import json
import pickle

import pytest
from dbos import SetWorkflowID

from drivers.durable_backend.dbos.adapter import init_backend
from drivers.durable_backend.restate.local_memo import (
    clear_memo,
    workflow_scope,
)
from drivers.durable_backend.restate.local_memo import (
    durable_infer as restate_infer,
)
from drivers.durable_backend.restate.local_memo import (
    durable_tool as restate_tool,
)
from drivers.durable_backend.restate.local_memo import (
    durable_workflow as restate_workflow,
)
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import BlockedNeedsGate, make_gated_tool_call, make_plain_tool_call
from trajectory_ir.resume.step import make_run_step
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import UnsafeInteger, UnsafePlan, payload_hash
from trajectory_ir.runtime.tool import Tool

TENANT_ID = "demo"
BIG = 2**53
BOUND = 2**53 - 1


def _kinds(node_log, traj):
    return [n["kind"] for n in node_log.list_nodes(traj, tenant_id=TENANT_ID)]


def _nodes(node_log, traj):
    return node_log.list_nodes(traj, tenant_id=TENANT_ID)


def _local_step(tmp_path, traj, tools):
    clear_memo()
    node_log = NodeLog(str(tmp_path / f"{traj}.sqlite"))
    run_step = make_run_step(
        node_log,
        TENANT_ID,
        traj,
        tools,
        durable_infer_fn=restate_infer,
        durable_tool_fn=restate_tool,
        durable_workflow_fn=restate_workflow,
    )
    return run_step, node_log


def _assert_no_wide_ints(value):
    if isinstance(value, bool):
        return
    if isinstance(value, int):
        assert abs(value) <= BOUND
        return
    if isinstance(value, dict):
        for item in value.values():
            _assert_no_wide_ints(item)
        return
    if isinstance(value, list):
        for item in value:
            _assert_no_wide_ints(item)


def _failure_row(node_log, traj, seq):
    for row in _nodes(node_log, traj):
        if row["kind"] == "TOOL_RESULT" and row["seq"] == seq:
            return row
    raise AssertionError(f"no TOOL_RESULT at seq={seq}")


@pytest.mark.parametrize("bad", [BIG, -BIG])
def test_plan_integer_is_not_memoized(tmp_path, bad):
    calls = {"model": 0, "tool": 0}

    def model(_context):
        calls["model"] += 1
        if calls["model"] == 1:
            return {"tool_calls": [{"name": "echo", "args": {"n": bad}}]}
        return {"tool_calls": [{"name": "echo", "args": {"n": 1}}]}

    def echo(n):
        calls["tool"] += 1
        return n

    tools = {"echo": Tool(name="echo", fn=echo, effect_class=EffectClass.PURE)}
    traj = f"plan-{bad}"
    run_step, node_log = _local_step(tmp_path, traj, tools)

    with workflow_scope(traj), pytest.raises(UnsafePlan, match=str(bad)):
        run_step(step_n=1, model_call=model, context={"goal": "demo"})
    assert calls == {"model": 1, "tool": 0}
    assert _kinds(node_log, traj) == ["PROJECT_CONTEXT"]

    with workflow_scope(traj):
        assert run_step(step_n=1, model_call=model, context={"goal": "demo"}) == [1]
    assert calls == {"model": 2, "tool": 1}
    assert "DECISION" in _kinds(node_log, traj)
    assert "COMMIT_STEP" in _kinds(node_log, traj)


def test_gated_result_integer_is_not_a_gate(tmp_path):
    calls = {"n": 0}

    def model(_context):
        return {"tool_calls": [{"name": "mint", "args": {}}]}

    def mint():
        calls["n"] += 1
        return {"id": BIG}

    tools = {
        "mint": Tool(
            name="mint",
            fn=mint,
            effect_class=EffectClass.NON_IDEMPOTENT_WRITE,
        )
    }
    traj = "gated-result"
    run_step, node_log = _local_step(tmp_path, traj, tools)

    with workflow_scope(traj), pytest.raises(UnsafeInteger, match=str(BIG)):
        run_step(step_n=1, model_call=model, context={})
    assert calls["n"] == 1
    assert _kinds(node_log, traj) == [
        "PROJECT_CONTEXT",
        "DECISION",
        "TOOL_CALL",
        "TOOL_RESULT",
    ]
    failed = _failure_row(node_log, traj, 3)
    assert failed["payload"]["error"] == "integer_domain"
    assert failed["payload"]["message"] == f"{BIG} exceeds safe integer domain for JSON floats"
    digest = failed["payload"]["result_digest"]
    assert (
        digest
        == hashlib.sha256(
            json.dumps({"id": BIG}, sort_keys=True, separators=(",", ":")).encode()
        ).hexdigest()
    )
    payload_hash(failed["payload"])
    _assert_no_wide_ints(failed["payload"])

    with workflow_scope(traj), pytest.raises(UnsafeInteger, match=str(BIG)):
        run_step(step_n=1, model_call=model, context={})
    assert calls["n"] == 1
    assert _kinds(node_log, traj) == [
        "PROJECT_CONTEXT",
        "DECISION",
        "TOOL_CALL",
        "TOOL_RESULT",
    ]
    assert "ABORT" not in _kinds(node_log, traj)
    assert "COMMIT_STEP" not in _kinds(node_log, traj)


def test_plain_result_integer_records_one_failure_node(tmp_path):
    calls = {"n": 0}

    def model(_context):
        return {"tool_calls": [{"name": "mint", "args": {}}]}

    def mint():
        calls["n"] += 1
        return {"id": BIG}

    tools = {"mint": Tool(name="mint", fn=mint, effect_class=EffectClass.PURE)}
    traj = "plain-result"
    run_step, node_log = _local_step(tmp_path, traj, tools)

    with workflow_scope(traj), pytest.raises(UnsafeInteger, match=str(BIG)):
        run_step(step_n=1, model_call=model, context={})
    first = _nodes(node_log, traj)
    assert [n["kind"] for n in first] == [
        "PROJECT_CONTEXT",
        "DECISION",
        "TOOL_CALL",
        "TOOL_RESULT",
    ]

    with workflow_scope(traj), pytest.raises(UnsafeInteger, match=str(BIG)):
        run_step(step_n=1, model_call=model, context={})
    again = _nodes(node_log, traj)
    assert [n["kind"] for n in again] == [n["kind"] for n in first]
    assert [n["id"] for n in again] == [n["id"] for n in first]
    assert "COMMIT_STEP" not in _kinds(node_log, traj)


def test_gated_args_are_rejected_before_the_body(tmp_path):
    calls = {"n": 0}
    log = NodeLog(str(tmp_path / "args.sqlite"))

    def deploy(n):
        calls["n"] += 1
        return n

    gated = make_gated_tool_call(log, "t", TENANT_ID, 1, 2, "deploy", deploy)
    with pytest.raises(UnsafeInteger, match=str(BIG)):
        gated(n=BIG)
    assert calls["n"] == 0
    assert log.list_nodes("t", tenant_id=TENANT_ID) == []


def test_plain_args_are_rejected_before_the_body(tmp_path):
    calls = {"n": 0}
    log = NodeLog(str(tmp_path / "plain-args.sqlite"))

    def echo(n):
        calls["n"] += 1
        return n

    plain = make_plain_tool_call(log, "t", TENANT_ID, 1, 2, "echo", echo)
    with pytest.raises(UnsafeInteger, match=str(BIG)):
        plain(n=BIG)
    assert calls["n"] == 0
    assert log.list_nodes("t", tenant_id=TENANT_ID) == []


def test_lost_claim_without_integer_failure_still_blocks(tmp_path):
    calls = {"n": 0}
    log = NodeLog(str(tmp_path / "crash.sqlite"))
    log.append(
        "TOOL_CALL",
        1,
        {"tool": "deploy", "args": {}},
        "t",
        TENANT_ID,
        2,
    )

    def deploy():
        calls["n"] += 1
        return "ok"

    gated = make_gated_tool_call(log, "t", TENANT_ID, 1, 2, "deploy", deploy)
    with pytest.raises(BlockedNeedsGate):
        gated()
    assert calls["n"] == 0
    assert log.has("t", TENANT_ID, 1, "ABORT", seq=3)


def test_direct_gated_result_resume_is_not_a_gate(tmp_path):
    calls = {"n": 0}
    log = NodeLog(str(tmp_path / "direct.sqlite"))

    def mint():
        calls["n"] += 1
        return {"id": BIG}

    gated = make_gated_tool_call(log, "t", TENANT_ID, 1, 2, "mint", mint)
    with pytest.raises(UnsafeInteger, match=str(BIG)):
        gated()
    with pytest.raises(UnsafeInteger, match=str(BIG)):
        gated()
    assert calls["n"] == 1
    kinds = [n["kind"] for n in log.list_nodes("t", tenant_id=TENANT_ID)]
    assert kinds == ["TOOL_CALL", "TOOL_RESULT"]


def test_safe_integer_and_float_results_still_commit(tmp_path):
    def model(_context):
        return {"tool_calls": [{"name": "echo", "args": {"n": BOUND}}]}

    def echo(n):
        return n

    tools = {"echo": Tool(name="echo", fn=echo, effect_class=EffectClass.PURE)}
    traj = "bound"
    run_step, node_log = _local_step(tmp_path, traj, tools)
    with workflow_scope(traj):
        assert run_step(step_n=1, model_call=model, context={}) == [BOUND]
    assert "COMMIT_STEP" in _kinds(node_log, traj)

    def model_float(_context):
        return {"tool_calls": [{"name": "echo", "args": {}}]}

    def echo_float():
        return float(BIG)

    tools_float = {"echo": Tool(name="echo", fn=echo_float, effect_class=EffectClass.PURE)}
    traj_float = "float-result"
    run_float, log_float = _local_step(tmp_path, traj_float, tools_float)
    with workflow_scope(traj_float):
        assert run_float(step_n=1, model_call=model_float, context={}) == [float(BIG)]
    assert "COMMIT_STEP" in _kinds(log_float, traj_float)


def test_unsafe_integer_survives_pickle():
    message = f"{BIG} exceeds safe integer domain for JSON floats"
    again = pickle.loads(pickle.dumps(UnsafeInteger(message)))
    assert type(again) is UnsafeInteger
    assert str(again) == message
    plan = pickle.loads(pickle.dumps(UnsafePlan(message)))
    assert type(plan) is UnsafePlan
    assert str(plan) == message
    assert plan.workflow_id is None


_DBOS_PLAN_CALLS = {"model": 0}
_DBOS_MINT_CALLS = {"n": 0}


def _dbos_plan_model(_context):
    _DBOS_PLAN_CALLS["model"] += 1
    if _DBOS_PLAN_CALLS["model"] == 1:
        return {"tool_calls": [{"name": "echo", "args": {"n": BIG}}]}
    return {"tool_calls": [{"name": "echo", "args": {"n": 1}}]}


def _dbos_echo(n):
    return n


def _dbos_mint_model(_context):
    return {"tool_calls": [{"name": "mint", "args": {}}]}


def _dbos_mint():
    _DBOS_MINT_CALLS["n"] += 1
    return {"id": BIG}


def test_dbos_retries_inference_when_the_plan_integer_is_unsafe(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    _DBOS_PLAN_CALLS["model"] = 0
    traj = "dbos-plan"
    node_log = NodeLog(str(tmp_path / "nodes.sqlite"))
    tools = {"echo": Tool(name="echo", fn=_dbos_echo, effect_class=EffectClass.PURE)}
    run_step = make_run_step(node_log, TENANT_ID, traj, tools)
    init_backend(db_path=str(tmp_path / "nodes.sqlite"))

    with SetWorkflowID(traj), pytest.raises(UnsafePlan, match=str(BIG)):
        run_step(step_n=1, model_call=_dbos_plan_model, context={})
    assert _DBOS_PLAN_CALLS["model"] == 1
    assert _kinds(node_log, traj) == ["PROJECT_CONTEXT"]

    with SetWorkflowID(traj):
        assert run_step(step_n=1, model_call=_dbos_plan_model, context={}) == [1]
    assert _DBOS_PLAN_CALLS["model"] == 2
    assert "COMMIT_STEP" in _kinds(node_log, traj)


def test_dbos_gated_result_integer_stays_a_storage_error(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    _DBOS_MINT_CALLS["n"] = 0
    traj = "dbos-gated"
    node_log = NodeLog(str(tmp_path / "gated.sqlite"))
    tools = {
        "mint": Tool(
            name="mint",
            fn=_dbos_mint,
            effect_class=EffectClass.NON_IDEMPOTENT_WRITE,
        )
    }
    run_step = make_run_step(node_log, TENANT_ID, traj, tools)
    init_backend(db_path=str(tmp_path / "gated.sqlite"))

    with SetWorkflowID(traj), pytest.raises(UnsafeInteger, match=str(BIG)):
        run_step(step_n=1, model_call=_dbos_mint_model, context={})
    assert _DBOS_MINT_CALLS["n"] == 1
    assert "TOOL_RESULT" in _kinds(node_log, traj)
    assert "ABORT" not in _kinds(node_log, traj)

    with SetWorkflowID(traj), pytest.raises(UnsafeInteger, match=str(BIG)):
        run_step(step_n=1, model_call=_dbos_mint_model, context={})
    assert _DBOS_MINT_CALLS["n"] == 1
    assert "ABORT" not in _kinds(node_log, traj)
    assert "COMMIT_STEP" not in _kinds(node_log, traj)
