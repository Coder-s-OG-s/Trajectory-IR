import pytest

from client.python.trajectory_client import (
    commit_step,
    exec_tool,
    open_trajectory,
    project,
    seal_decision,
)
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool


@pytest.fixture
def db_path(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    return str(tmp_path / "test_client.sqlite")


def test_project_appends_project_context_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t1", db_path=db_path)
    project(traj, step_n=1, context={"foo": "bar"})
    assert NodeLog(db_path).has("test-t1", "demo", 1, "PROJECT_CONTEXT")


def test_seal_decision_appends_decision_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t2", db_path=db_path)
    seal_decision(traj, step_n=1, plan={"tool_calls": []})
    assert NodeLog(db_path).has("test-t2", "demo", 1, "DECISION")


def test_exec_tool_runs_idempotent_write_directly(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t3", db_path=db_path)
    tool = Tool(name="noop", fn=lambda x: x + 1, effect_class=EffectClass.IDEMPOTENT_WRITE)
    result = exec_tool(traj, step_n=1, call={"args": {"x": 1}}, tool=tool, seq=2)
    assert result.result == 2


def test_commit_step_appends_commit_step_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t4", db_path=db_path)
    commit_step(traj, step_n=1, seq=2)
    assert NodeLog(db_path).has("test-t4", "demo", 1, "COMMIT_STEP")


def test_exec_tool_two_non_idempotent_writes_same_step_distinct_seq(db_path):
    """Regression test: two NON_IDEMPOTENT_WRITE calls in same step should both succeed
    if they use distinct seq values, not falsely block the second as a duplicate."""
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t5", db_path=db_path)

    # First tool call with seq=2
    tool1 = Tool(name="add", fn=lambda x: x + 1, effect_class=EffectClass.NON_IDEMPOTENT_WRITE)
    result1 = exec_tool(traj, step_n=1, call={"args": {"x": 1}}, tool=tool1, seq=2)
    assert result1.result == 2

    # Second tool call with seq=4 (following the 2 + 2*i pattern)
    tool2 = Tool(name="mul", fn=lambda x: x * 2, effect_class=EffectClass.NON_IDEMPOTENT_WRITE)
    result2 = exec_tool(traj, step_n=1, call={"args": {"x": 3}}, tool=tool2, seq=4)
    assert result2.result == 6

    # Both should be logged without either being falsely blocked
    assert NodeLog(db_path).has("test-t5", "demo", 1, "TOOL_CALL", seq=2)
    assert NodeLog(db_path).has("test-t5", "demo", 1, "TOOL_CALL", seq=4)
