"""Tests for Restate adapter local memo and make_run_step injection."""

from __future__ import annotations

import pytest

from drivers.durable_backend.restate import (
    durable_infer,
    durable_tool,
    durable_workflow,
    init_backend,
)
from drivers.durable_backend.restate.local_memo import clear_memo, workflow_scope
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.step import make_run_step
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool

TRAJECTORY_ID = "restate-local-demo"
TENANT_ID = "demo"


def test_local_memo_infer_runs_once():
    clear_memo()
    init_backend()
    calls = {"n": 0}

    def model(context: dict) -> dict:
        calls["n"] += 1
        return {"tool_calls": [], "v": context.get("v")}

    wrapped = durable_infer(model)
    with workflow_scope("wf-1"):
        a = wrapped({"v": 1})
        b = wrapped({"v": 1})
    assert a == b
    assert calls["n"] == 1


def test_local_memo_tool_runs_once():
    clear_memo()
    init_backend()
    calls = {"n": 0}

    def add(*, x: int) -> int:
        calls["n"] += 1
        return x + 1

    wrapped = durable_tool(add)
    with workflow_scope("wf-2"):
        assert wrapped(x=1) == 2
        assert wrapped(x=1) == 2
    assert calls["n"] == 1


def test_make_run_step_with_restate_local_memo(tmp_path):
    clear_memo()
    init_backend()
    model_calls = {"n": 0}
    tool_calls = {"n": 0}

    def model_call(context: dict) -> dict:
        model_calls["n"] += 1
        return {"tool_calls": [{"name": "echo", "args": {"msg": "hi"}}]}

    def echo(*, msg: str) -> str:
        tool_calls["n"] += 1
        return msg

    node_log = NodeLog(str(tmp_path / "nodes.sqlite"))
    tools = {
        "echo": Tool(name="echo", fn=echo, effect_class=EffectClass.PURE),
    }
    run_step = make_run_step(
        node_log,
        TENANT_ID,
        TRAJECTORY_ID,
        tools,
        durable_infer_fn=durable_infer,
        durable_tool_fn=durable_tool,
        durable_workflow_fn=durable_workflow,
    )

    with workflow_scope(TRAJECTORY_ID):
        r1 = run_step(step_n=1, model_call=model_call, context={})
        r2 = run_step(step_n=1, model_call=model_call, context={})

    assert r1 == ["hi"]
    assert r2 == ["hi"]
    # Durable memo: model and tool body run once across replay.
    assert model_calls["n"] == 1
    assert tool_calls["n"] == 1
    assert node_log.has(TRAJECTORY_ID, TENANT_ID, 1, "DECISION")
    assert node_log.has(TRAJECTORY_ID, TENANT_ID, 1, "TOOL_CALL", seq=2)
    assert node_log.has(TRAJECTORY_ID, TENANT_ID, 1, "COMMIT_STEP")


def test_partial_durable_injection_fails_loud(tmp_path):
    """Partial hooks would mix backends; refuse instead of filling from DBOS."""
    node_log = NodeLog(str(tmp_path / "nodes.sqlite"))
    tools = {
        "echo": Tool(name="echo", fn=lambda *, msg: msg, effect_class=EffectClass.PURE),
    }
    with pytest.raises(ValueError, match="must be injected together"):
        make_run_step(
            node_log,
            TENANT_ID,
            TRAJECTORY_ID,
            tools,
            durable_infer_fn=durable_infer,
            # durable_tool_fn / durable_workflow_fn omitted on purpose
        )


def test_partial_durable_injection_lists_missing_names(tmp_path):
    node_log = NodeLog(str(tmp_path / "nodes.sqlite"))
    tools = {
        "echo": Tool(name="echo", fn=lambda *, msg: msg, effect_class=EffectClass.PURE),
    }
    with pytest.raises(ValueError, match="durable_workflow_fn"):
        make_run_step(
            node_log,
            TENANT_ID,
            "partial-2",
            tools,
            durable_infer_fn=durable_infer,
            durable_tool_fn=durable_tool,
        )
