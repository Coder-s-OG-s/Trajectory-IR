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
from trajectory_ir.resume.step import make_run_step
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool

TRAJECTORY_ID = "test-step-plain-tool"
TENANT_ID = "demo"


def _echo(msg):
    return msg


def _model_call(context):
    return {"tool_calls": [{"name": "echo", "args": {"msg": "hello"}}]}


def test_plain_tool_logs_tool_call_and_result(tmp_path, monkeypatch):
    """Issue #45: PURE/READ_ONLY/IDEMPOTENT_WRITE tools must still append
    TOOL_CALL/TOOL_RESULT nodes, matching Go's RunStep. Only the
    NON_IDEMPOTENT_WRITE gate path is exempt from needing extra logging
    because make_gated_tool_call already logs both nodes itself."""
    monkeypatch.chdir(tmp_path)

    node_log = NodeLog(str(tmp_path / "nodes.sqlite"))

    tool_registry = {
        "echo": Tool(
            name="echo",
            fn=_echo,
            effect_class=EffectClass.PURE,
        ),
    }

    run_step = make_run_step(node_log, TENANT_ID, TRAJECTORY_ID, tool_registry)

    init_backend(app_name="test-step-plain-tool")

    with SetWorkflowID(TRAJECTORY_ID):
        results = run_step(step_n=1, model_call=_model_call, context={})

    assert results == ["hello"]
    assert node_log.has(TRAJECTORY_ID, TENANT_ID, 1, "TOOL_CALL", seq=2)
    assert node_log.has(TRAJECTORY_ID, TENANT_ID, 1, "TOOL_RESULT", seq=3)


def _failing_tool():
    raise RuntimeError("tool error")


def _model_call_fail(context):
    return {"tool_calls": [{"name": "fail", "args": {}}]}


def test_plain_tool_logs_tool_call_when_tool_fails(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    node_log = NodeLog(str(tmp_path / "nodes_fail.sqlite"))

    tool_registry = {
        "fail": Tool(
            name="fail",
            fn=_failing_tool,
            effect_class=EffectClass.READ_ONLY,
        ),
    }

    run_step = make_run_step(node_log, TENANT_ID, "test-step-fail", tool_registry)
    init_backend(app_name="test-step-fail")

    with SetWorkflowID("test-step-fail"), pytest.raises(RuntimeError, match="tool error"):
        run_step(step_n=1, model_call=_model_call_fail, context={})

    assert node_log.has("test-step-fail", TENANT_ID, 1, "TOOL_CALL", seq=2)
    assert not node_log.has("test-step-fail", TENANT_ID, 1, "TOOL_RESULT", seq=3)


_REPLAY_CALLS = {"n": 0}


def _replay_tool(x="a"):
    _REPLAY_CALLS["n"] += 1
    return f"result-{x}"


def _model_call_replay(context):
    return {"tool_calls": [{"name": "count", "args": {"x": "a"}}]}


def test_plain_tool_replay_does_not_duplicate_nodes(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    node_log = NodeLog(str(tmp_path / "nodes_replay.sqlite"))
    traj_id = "test-step-replay-nodup"
    _REPLAY_CALLS["n"] = 0

    tool_registry = {
        "count": Tool(name="count", fn=_replay_tool, effect_class=EffectClass.PURE),
    }

    run_step = make_run_step(node_log, TENANT_ID, traj_id, tool_registry)
    init_backend(app_name="test-step-replay")

    with SetWorkflowID(traj_id):
        r1 = run_step(step_n=1, model_call=_model_call_replay, context={})
    assert r1 == ["result-a"]
    assert _REPLAY_CALLS["n"] == 1

    # Replay under same workflow ID
    with SetWorkflowID(traj_id):
        r2 = run_step(step_n=1, model_call=_model_call_replay, context={})
    assert r2 == ["result-a"]
    assert _REPLAY_CALLS["n"] == 1

    nodes = node_log.list_nodes(traj_id, tenant_id=TENANT_ID)
    # Expected exactly: PROJECT_CONTEXT, DECISION, TOOL_CALL, TOOL_RESULT, COMMIT_STEP
    assert len(nodes) == 5


class CustomToolError(Exception):
    pass


_REPLAY_FAIL_CALLS = {"n": 0}


def _replay_failing_tool():
    _REPLAY_FAIL_CALLS["n"] += 1
    if _REPLAY_FAIL_CALLS["n"] == 1:
        raise CustomToolError("timeout after 100ms")
    raise CustomToolError("timeout after 150ms")


def _model_call_replay_fail(context):
    return {"tool_calls": [{"name": "fail", "args": {}}]}


def test_plain_tool_failure_replay_does_not_conflict(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    node_log = NodeLog(str(tmp_path / "nodes_fail_replay.sqlite"))
    traj_id = "test-step-fail-replay"
    _REPLAY_FAIL_CALLS["n"] = 0

    tool_registry = {
        "fail": Tool(name="fail", fn=_replay_failing_tool, effect_class=EffectClass.READ_ONLY),
    }

    clear_memo()
    run_step = make_run_step(
        node_log,
        TENANT_ID,
        traj_id,
        tool_registry,
        durable_infer_fn=restate_infer,
        durable_tool_fn=restate_tool,
        durable_workflow_fn=restate_workflow,
    )

    with workflow_scope(traj_id), pytest.raises(CustomToolError, match="timeout after 100ms"):
        run_step(step_n=1, model_call=_model_call_replay_fail, context={})

    # Retry with different error text: surfaces second error without slot conflict
    with workflow_scope(traj_id), pytest.raises(CustomToolError, match="timeout after 150ms"):
        run_step(step_n=1, model_call=_model_call_replay_fail, context={})

    nodes = node_log.list_nodes(traj_id, tenant_id=TENANT_ID)
    # Expected exactly: PROJECT_CONTEXT, DECISION, TOOL_CALL (no TOOL_RESULT on error)
    assert len(nodes) == 3
