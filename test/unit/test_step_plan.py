import pytest

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

TENANT_ID = "demo"


def _echo(**args):
    return args


def _kinds(node_log, traj):
    return [n["kind"] for n in node_log.list_nodes(traj, tenant_id=TENANT_ID)]


def _step(tmp_path, traj, tools=None, on_sealed=None):
    clear_memo()
    node_log = NodeLog(str(tmp_path / f"{traj}.sqlite"))
    if tools is None:
        tools = {"echo": Tool(name="echo", fn=_echo, effect_class=EffectClass.PURE)}
    run_step = make_run_step(
        node_log,
        TENANT_ID,
        traj,
        tools,
        on_decision_sealed=on_sealed,
        durable_infer_fn=restate_infer,
        durable_tool_fn=restate_tool,
        durable_workflow_fn=restate_workflow,
    )
    return run_step, node_log


def _call(run_step, traj, plan):
    def model(_context):
        return plan

    with workflow_scope(traj):
        return run_step(step_n=1, model_call=model, context={"goal": "demo"})


@pytest.mark.parametrize(
    ("plan", "match"),
    [
        ({}, "missing tool_calls"),
        ({"tool_calls": "echo"}, "must be a list"),
        ({"tool_calls": [{"args": {}}]}, "missing name"),
        ({"tool_calls": [{"name": "ghost", "args": {}}]}, 'unknown tool "ghost"'),
        ({"tool_calls": [{"name": "echo", "args": ["x"]}]}, r"tool_calls\[0\]\.args"),
    ],
)
def test_bad_plan_raises_after_decision(tmp_path, plan, match):
    sealed = {"n": 0}

    def on_sealed():
        sealed["n"] += 1

    traj = "bad-plan"
    run_step, node_log = _step(tmp_path, traj, on_sealed=on_sealed)
    with pytest.raises(ValueError, match=match):
        _call(run_step, traj, plan)
    assert _kinds(node_log, traj) == ["PROJECT_CONTEXT", "DECISION"]
    assert sealed["n"] == 1


def test_non_object_plan_is_rejected_before_decision(tmp_path):
    sealed = {"n": 0}

    def on_sealed():
        sealed["n"] += 1

    traj = "plan-list"
    run_step, node_log = _step(tmp_path, traj, on_sealed=on_sealed)
    with pytest.raises(ValueError, match="want object"):
        _call(run_step, traj, ["echo"])
    assert _kinds(node_log, traj) == ["PROJECT_CONTEXT"]
    assert sealed["n"] == 0


def test_missing_args_calls_tool_with_empty_map(tmp_path):
    seen = {}

    def echo(**args):
        seen["args"] = args
        return "ok"

    tools = {"echo": Tool(name="echo", fn=echo, effect_class=EffectClass.PURE)}
    run_step, node_log = _step(tmp_path, "no-args", tools=tools)
    results = _call(run_step, "no-args", {"tool_calls": [{"name": "echo"}]})
    assert results == ["ok"]
    assert seen["args"] == {}
    assert _kinds(node_log, "no-args") == [
        "PROJECT_CONTEXT",
        "DECISION",
        "TOOL_CALL",
        "TOOL_RESULT",
        "COMMIT_STEP",
    ]


def test_missing_args_calls_gated_tool_with_empty_map(tmp_path):
    seen = {}

    def deploy(**args):
        seen["args"] = args
        return "deployed"

    tools = {
        "deploy": Tool(
            name="deploy",
            fn=deploy,
            effect_class=EffectClass.NON_IDEMPOTENT_WRITE,
        )
    }
    run_step, node_log = _step(tmp_path, "gated-no-args", tools=tools)
    results = _call(run_step, "gated-no-args", {"tool_calls": [{"name": "deploy"}]})
    assert results == ["deployed"]
    assert seen["args"] == {}
    assert "TOOL_CALL" in _kinds(node_log, "gated-no-args")
    assert "COMMIT_STEP" in _kinds(node_log, "gated-no-args")
