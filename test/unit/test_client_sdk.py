import os
import tempfile

import pytest

from client.python.trajectory_client import (
    open_trajectory, project, seal_decision, exec_tool, commit_step,
)
from trajectory_ir.runtime.tool import Tool
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.log import NodeLog


@pytest.fixture
def db_path(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    return str(tmp_path / "test_client.sqlite")


def test_project_appends_project_context_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t1", db_path=db_path)
    project(traj, step_n=1, context={"foo": "bar"})
    assert NodeLog(db_path).has("test-t1", 1, "PROJECT_CONTEXT")


def test_seal_decision_appends_decision_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t2", db_path=db_path)
    seal_decision(traj, step_n=1, plan={"tool_calls": []})
    assert NodeLog(db_path).has("test-t2", 1, "DECISION")


def test_exec_tool_runs_idempotent_write_directly(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t3", db_path=db_path)
    tool = Tool(name="noop", fn=lambda x: x + 1, effect_class=EffectClass.IDEMPOTENT_WRITE)
    result = exec_tool(traj, step_n=1, call={"args": {"x": 1}}, tool=tool)
    assert result.result == 2


def test_commit_step_appends_commit_step_node(db_path):
    traj = open_trajectory(tenant_id="demo", trajectory_id="test-t4", db_path=db_path)
    commit_step(traj, step_n=1)
    assert NodeLog(db_path).has("test-t4", 1, "COMMIT_STEP")
