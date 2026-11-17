"""Tests for Restate adapter local memo and make_run_step injection."""

from __future__ import annotations

import pytest

from trajectory_ir.drivers.durable_backend.restate import (
    durable_infer,
    durable_tool,
    durable_workflow,
    init_backend,
    local_memo,
)
from trajectory_ir.drivers.durable_backend.restate.local_memo import clear_memo, workflow_scope
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
    with workflow_scope("wf-1"):
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
    with workflow_scope("wf-2"):
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
    with workflow_scope(TRAJECTORY_ID):
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


def test_local_memo_tool_sequence_in_same_scope():
    clear_memo()
    init_backend()
    calls = {"n": 0}

    def add(*, x: int) -> int:
        calls["n"] += 1
        return x + 1

    wrapped = durable_tool(add)
    with workflow_scope("wf-3"):
        assert wrapped(x=1) == 2
        assert wrapped(x=1) == 2
    assert calls["n"] == 2


def test_local_memo_cross_step_isolation(tmp_path):
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

    node_log = NodeLog(str(tmp_path / "nodes2.sqlite"))
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
    with workflow_scope(TRAJECTORY_ID):
        r2 = run_step(step_n=2, model_call=model_call, context={})

    assert r1 == ["hi"]
    assert r2 == ["hi"]
    # Two different steps should run independently without memo collisions
    assert model_calls["n"] == 2
    assert tool_calls["n"] == 2


def test_local_memo_rejects_values_json_cannot_encode():
    clear_memo()
    init_backend()
    calls: list[object] = []

    def take(x: object) -> object:
        calls.append(x)
        return x

    wrapped = durable_tool(take)

    class Box:
        def __str__(self) -> str:
            return "box"

    with workflow_scope("bad"):
        with pytest.raises(ValueError, match="JSON-serializable"):
            wrapped(Box())
        assert wrapped(1) == 1
    with workflow_scope("bad"):
        assert wrapped(1) == 1
    assert calls == [1]

    with workflow_scope("bytes"):
        with pytest.raises(ValueError, match="JSON-serializable"):
            wrapped(b"ab")
        with pytest.raises(ValueError, match="JSON-serializable"):
            wrapped({"k": {1, 2}})
    loop: list[object] = []
    loop.append(loop)
    with workflow_scope("loop"), pytest.raises(ValueError, match="JSON-serializable"):
        wrapped(loop)
    assert calls == [1]


def test_local_memo_tuple_does_not_replay_a_list():
    clear_memo()
    init_backend()
    calls: list[str] = []

    def kind(x: object) -> str:
        calls.append(type(x).__name__)
        return type(x).__name__

    wrapped = durable_tool(kind)
    with workflow_scope("seq"):
        assert wrapped([1, 2]) == "list"
    with workflow_scope("seq"), pytest.raises(ValueError, match="JSON-serializable"):
        wrapped((1, 2))
    with workflow_scope("seq"):
        assert wrapped([1, 2]) == "list"
    assert calls == ["list"]

    positional: list[tuple[int, int]] = []

    def add(x: int, y: int) -> int:
        positional.append((x, y))
        return x + y

    summed = durable_tool(add)
    with workflow_scope("pos"):
        assert summed(1, 2) == 3
    with workflow_scope("pos"):
        assert summed(1, 2) == 3
    assert positional == [(1, 2)]


def test_local_memo_kwargs_order_still_hits():
    clear_memo()
    init_backend()
    calls: list[dict[str, int]] = []

    def add(**kwargs: int) -> int:
        calls.append(dict(kwargs))
        return kwargs["a"] + kwargs["b"]

    wrapped = durable_tool(add)
    with workflow_scope("kw"):
        assert wrapped(a=1, b=2) == 3
    with workflow_scope("kw"):
        assert wrapped(b=2, a=1) == 3
    assert calls == [{"a": 1, "b": 2}]


def test_local_memo_drops_least_recently_used(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setattr(local_memo, "_MEMO_LIMIT", 2)
    clear_memo()
    init_backend()
    calls: list[int] = []

    def add(*, x: int) -> int:
        calls.append(x)
        return x

    wrapped = durable_tool(add)
    with workflow_scope("a"):
        assert wrapped(x=1) == 1
    with workflow_scope("b"):
        assert wrapped(x=2) == 2
    with workflow_scope("a"):
        assert wrapped(x=1) == 1
    assert calls == [1, 2]
    with workflow_scope("c"):
        assert wrapped(x=3) == 3
    assert len(local_memo._memo) == 2
    with workflow_scope("a"):
        assert wrapped(x=1) == 1
    with workflow_scope("b"):
        assert wrapped(x=2) == 2
    assert calls == [1, 2, 3, 2]

    clear_memo()
    calls.clear()
    with workflow_scope("long"):
        for i in range(5):
            assert wrapped(x=i) == i
    assert calls == [0, 1, 2, 3, 4]
    assert len(local_memo._memo) == 2
    init_backend()
    assert len(local_memo._memo) == 2
