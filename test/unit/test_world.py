"""Spec §8.4: optional WORLD_SNAPSHOT, fail-loud WORLD_DRIFT."""

import pytest

from client.python.trajectory_client import (
    WorldDrift,
    check_world,
    open_trajectory,
    project,
    seal_decision,
)
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.step import make_run_step
from trajectory_ir.resume.world import decision_payload
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool


def test_decision_payload_omits_empty_snapshot():
    assert decision_payload({"tool_calls": []}) == {"plan": {"tool_calls": []}}
    assert decision_payload({"tool_calls": []}, {}) == {"plan": {"tool_calls": []}}


def test_decision_payload_includes_host_snapshot():
    payload = decision_payload({"tool_calls": []}, {"cluster_generation": "42"})
    assert payload["world_snapshot"] == {"cluster_generation": "42"}


def test_check_world_noop_without_snapshot(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    traj = open_trajectory(tenant_id="demo", trajectory_id="w1", db_path=str(tmp_path / "n.sqlite"))
    project(traj, 1, {"goal": "x"})
    seal_decision(traj, 1, {"tool_calls": []})
    check_world(traj, 1, {"cluster_generation": "99"})


def test_check_world_passes_when_observation_matches(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    traj = open_trajectory(tenant_id="demo", trajectory_id="w2", db_path=str(tmp_path / "n.sqlite"))
    project(traj, 1, {})
    seal_decision(
        traj,
        1,
        {"tool_calls": []},
        world_snapshot={"cluster_generation": "42", "node": "i-123"},
    )
    check_world(traj, 1, {"cluster_generation": "42", "node": "i-123", "extra": "ignored"})


def test_check_world_fail_loud_on_drift(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    traj = open_trajectory(tenant_id="demo", trajectory_id="w3", db_path=str(tmp_path / "n.sqlite"))
    project(traj, 1, {})
    seal_decision(traj, 1, {"tool_calls": []}, world_snapshot={"cluster_generation": "42"})
    with pytest.raises(WorldDrift, match="WORLD_DRIFT") as exc:
        check_world(traj, 1, {"cluster_generation": "43"})
    assert exc.value.step_n == 1
    assert exc.value.mismatches[0]["key"] == "cluster_generation"


def test_check_world_fail_loud_on_missing_key(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    traj = open_trajectory(tenant_id="demo", trajectory_id="w4", db_path=str(tmp_path / "n.sqlite"))
    project(traj, 1, {})
    seal_decision(traj, 1, {"tool_calls": []}, world_snapshot={"node": "i-123"})
    with pytest.raises(WorldDrift, match="WORLD_DRIFT"):
        check_world(traj, 1, {})


def test_check_world_requires_decision(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    traj = open_trajectory(tenant_id="demo", trajectory_id="w5", db_path=str(tmp_path / "n.sqlite"))
    with pytest.raises(ValueError, match="no DECISION"):
        check_world(traj, 1, {"k": "v"})


def _echo_msg(msg):
    return msg


def _world_model(_context):
    return {"tool_calls": [{"name": "echo", "args": {"msg": "ok"}}]}


def test_run_step_observe_world_drift(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    from dbos import SetWorkflowID

    from drivers.durable_backend.dbos.adapter import init_backend

    db = str(tmp_path / "nodes.sqlite")
    log = NodeLog(db)
    init_backend(db_path=db)
    tools = {
        "echo": Tool(name="echo", fn=_echo_msg, effect_class=EffectClass.PURE),
    }
    observed = {"cluster_generation": "1"}

    def observe():
        return dict(observed)

    run_step = make_run_step(
        log,
        "demo",
        "w-run",
        tools,
        world_snapshot={"cluster_generation": "1"},
        observe_world=observe,
    )
    with SetWorkflowID("w-run"):
        assert run_step(step_n=1, model_call=_world_model, context={}) == ["ok"]

    observed["cluster_generation"] = "2"
    with SetWorkflowID("w-run-2"), pytest.raises(WorldDrift, match="WORLD_DRIFT"):
        run_step(step_n=2, model_call=_world_model, context={})


def _passthrough(fn):
    return fn


def test_world_snapshot_callable_is_per_step(tmp_path):
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    current = {"step": 1}

    def snap(step_n: int):
        return {"cluster_generation": str(step_n)}

    def observe():
        return {"cluster_generation": str(current["step"])}

    tools = {
        "echo": Tool(name="echo", fn=_echo_msg, effect_class=EffectClass.PURE),
    }
    run_step = make_run_step(
        log,
        "demo",
        "w-per-step",
        tools,
        world_snapshot=snap,
        observe_world=observe,
        durable_infer_fn=_passthrough,
        durable_tool_fn=_passthrough,
        durable_workflow_fn=_passthrough,
    )
    assert run_step(step_n=1, model_call=_world_model, context={}) == ["ok"]
    current["step"] = 2
    assert run_step(step_n=2, model_call=_world_model, context={}) == ["ok"]
    decisions = {
        row["step_n"]: row["payload"]["world_snapshot"]
        for row in log.list_nodes("w-per-step", tenant_id="demo")
        if row["kind"] == "DECISION"
    }
    assert decisions == {1: {"cluster_generation": "1"}, 2: {"cluster_generation": "2"}}


def test_resume_after_decision_reports_drift_not_slot_conflict(tmp_path):
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    state = {"gen": "1", "observes": 0}

    def snap(_step_n: int):
        return {"cluster_generation": state["gen"]}

    def observe():
        state["observes"] += 1
        if state["observes"] == 1:
            raise RuntimeError("crash after decision")
        return {"cluster_generation": state["gen"]}

    tools = {
        "echo": Tool(name="echo", fn=_echo_msg, effect_class=EffectClass.PURE),
    }
    run_step = make_run_step(
        log,
        "demo",
        "w-resume",
        tools,
        world_snapshot=snap,
        observe_world=observe,
        durable_infer_fn=_passthrough,
        durable_tool_fn=_passthrough,
        durable_workflow_fn=_passthrough,
    )
    with pytest.raises(RuntimeError, match="crash after decision"):
        run_step(step_n=1, model_call=_world_model, context={})
    state["gen"] = "2"
    with pytest.raises(WorldDrift, match="WORLD_DRIFT") as exc:
        run_step(step_n=1, model_call=_world_model, context={})
    assert exc.value.mismatches[0]["sealed"] == "1"
    assert exc.value.mismatches[0]["observed"] == "2"
