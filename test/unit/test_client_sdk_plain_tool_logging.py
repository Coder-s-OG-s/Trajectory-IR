import pytest

from client.python.trajectory_client import exec_tool, open_trajectory
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool


def test_exec_tool_logs_tool_call_and_result_for_non_gated_tool(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db_path = str(tmp_path / "test_plain_tool_logging.sqlite")
    traj = open_trajectory(tenant_id="demo", trajectory_id="plain-1", db_path=db_path)

    tool = Tool(name="double", fn=lambda x: x * 2, effect_class=EffectClass.IDEMPOTENT_WRITE)
    result = exec_tool(traj, step_n=1, call={"args": {"x": 5}}, tool=tool, seq=2)

    assert result.result == 10
    log = NodeLog(db_path)
    assert log.has("plain-1", "demo", 1, "TOOL_CALL", seq=2)
    assert log.has("plain-1", "demo", 1, "TOOL_RESULT", seq=3)


def test_exec_tool_logs_two_plain_tools_same_step_distinct_seq(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db_path = str(tmp_path / "test_plain_tool_logging_multi.sqlite")
    traj = open_trajectory(tenant_id="demo", trajectory_id="plain-2", db_path=db_path)

    tool_a = Tool(name="incr", fn=lambda x: x + 1, effect_class=EffectClass.PURE)
    tool_b = Tool(name="square", fn=lambda x: x * x, effect_class=EffectClass.PURE)

    exec_tool(traj, step_n=1, call={"args": {"x": 1}}, tool=tool_a, seq=2)
    exec_tool(traj, step_n=1, call={"args": {"x": 3}}, tool=tool_b, seq=4)

    log = NodeLog(db_path)
    assert log.has("plain-2", "demo", 1, "TOOL_CALL", seq=2)
    assert log.has("plain-2", "demo", 1, "TOOL_RESULT", seq=3)
    assert log.has("plain-2", "demo", 1, "TOOL_CALL", seq=4)
    assert log.has("plain-2", "demo", 1, "TOOL_RESULT", seq=5)


def test_exec_tool_logs_tool_call_when_tool_raises(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db_path = str(tmp_path / "test_plain_tool_err.sqlite")
    traj = open_trajectory(tenant_id="demo", trajectory_id="plain-err", db_path=db_path)

    def failing_fn(x):
        raise ValueError("boom")

    tool = Tool(name="fail", fn=failing_fn, effect_class=EffectClass.PURE)
    with pytest.raises(ValueError, match="boom"):
        exec_tool(traj, step_n=1, call={"args": {"x": 1}}, tool=tool, seq=2)

    log = NodeLog(db_path)
    assert log.has("plain-err", "demo", 1, "TOOL_CALL", seq=2)
    assert not log.has("plain-err", "demo", 1, "TOOL_RESULT", seq=3)
