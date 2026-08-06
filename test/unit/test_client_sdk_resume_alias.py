"""Regression test for issue #97: client SDK resume() must be a real, tested alias.

resume() is a thin wrapper around the same Trajectory construction
open_trajectory() does, by design (see its docstring for why). This test
covers what actually distinguishes it: rejecting resume of a trajectory with
no prior history, and reattaching correctly to continue a step across a
simulated crash/restart without re-running an already-claimed gated tool call.
"""

from __future__ import annotations

import pytest

from client.python.trajectory_client import exec_tool, open_trajectory, resume, seal_decision
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import BlockedNeedsGate
from trajectory_ir.runtime.tool import Tool


def _deploy_tool(calls: list[str]) -> Tool:
    def deploy_server(version: str) -> dict:
        calls.append(version)
        return {"deployed": version}

    return Tool(
        name="deploy_server", fn=deploy_server, effect_class=EffectClass.NON_IDEMPOTENT_WRITE
    )


def test_resume_raises_on_trajectory_with_no_history(tmp_path):
    db_path = str(tmp_path / "trajectory.sqlite")

    with pytest.raises(ValueError, match="no existing nodes"):
        resume("brand-new-trajectory", db_path=db_path)


def test_resume_reattaches_after_history_exists(tmp_path):
    db_path = str(tmp_path / "trajectory.sqlite")
    trajectory = open_trajectory("demo", "t1", db_path=db_path)
    seal_decision(trajectory, step_n=1, plan={"tool_calls": []})

    resumed = resume("t1", db_path=db_path)

    assert resumed.trajectory_id == "t1"
    assert resumed.tenant_id == "demo"
    assert resumed.db_path == db_path


def test_resume_does_not_replay_already_claimed_gated_tool_call(tmp_path):
    """Simulates a crash/restart mid-step: resume() reattaches, and re-driving
    the same exec_tool call for a NON_IDEMPOTENT_WRITE tool blocks instead of
    re-running the side effect (README S8, R02)."""
    db_path = str(tmp_path / "trajectory.sqlite")
    calls: list[str] = []
    tool = _deploy_tool(calls)

    trajectory = open_trajectory("demo", "t1", db_path=db_path)
    seal_decision(
        trajectory,
        step_n=1,
        plan={"tool_calls": [{"name": "deploy_server", "args": {"version": "1.0.0"}}]},
    )
    exec_tool(trajectory, step_n=1, call={"args": {"version": "1.0.0"}}, tool=tool, seq=2)
    assert calls == ["1.0.0"]

    # Simulate crash/restart: drop the in-process trajectory and reattach.
    del trajectory
    resumed = resume("t1", db_path=db_path)

    with pytest.raises(BlockedNeedsGate):
        exec_tool(resumed, step_n=1, call={"args": {"version": "1.0.0"}}, tool=tool, seq=2)

    # The already-claimed slot blocks retry; the tool body must not re-run.
    assert calls == ["1.0.0"]
