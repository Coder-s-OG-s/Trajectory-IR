"""Conformance R06: sandbox rejects NON_IDEMPOTENT_WRITE, AGENT_SPAWN, SENSITIVE."""

from __future__ import annotations

import pytest

from client.python.trajectory_client import exec_tool, open_trajectory
from trajectory_ir.effects import EffectClass
from trajectory_ir.runtime.sandbox import RunMode, SandboxForbidden
from trajectory_ir.runtime.tool import Tool


def test_r06_sandbox_rejects_non_idempotent(tmp_path) -> None:
    side = {"n": 0}

    def deploy(version: str = "1") -> dict:
        side["n"] += 1
        return {"deployed": version}

    traj = open_trajectory(
        "demo",
        "r06-sandbox",
        db_path=str(tmp_path / "t.sqlite"),
        mode=RunMode.SANDBOX,
    )
    tool = Tool(name="deploy_server", fn=deploy, effect_class=EffectClass.NON_IDEMPOTENT_WRITE)
    with pytest.raises(SandboxForbidden) as ei:
        exec_tool(traj, 1, {"args": {"version": "1"}}, tool, seq=2)
    assert "SANDBOX_FORBIDDEN" in str(ei.value)
    assert side["n"] == 0


def test_r06_sandbox_rejects_agent_spawn(tmp_path) -> None:
    side = {"n": 0}

    def spawn_worker(task: str = "") -> dict:
        side["n"] += 1
        return {"spawned": task}

    traj = open_trajectory(
        "demo",
        "r06-spawn",
        db_path=str(tmp_path / "t.sqlite"),
        mode=RunMode.SANDBOX,
    )
    tool = Tool(name="spawn_worker", fn=spawn_worker, effect_class=EffectClass.AGENT_SPAWN)
    with pytest.raises(SandboxForbidden) as ei:
        exec_tool(traj, 1, {"args": {"task": "work"}}, tool, seq=2)
    assert "SANDBOX_FORBIDDEN" in str(ei.value)
    assert side["n"] == 0


def test_r06_sandbox_rejects_sensitive(tmp_path) -> None:
    side = {"n": 0}

    def read_api_key(name: str = "") -> str:
        side["n"] += 1
        return "secret"

    traj = open_trajectory(
        "demo",
        "r06-sensitive",
        db_path=str(tmp_path / "t.sqlite"),
        mode=RunMode.SANDBOX,
    )
    tool = Tool(name="read_api_key", fn=read_api_key, effect_class=EffectClass.SENSITIVE)
    with pytest.raises(SandboxForbidden) as ei:
        exec_tool(traj, 1, {"args": {"name": "key"}}, tool, seq=2)
    assert "SANDBOX_FORBIDDEN" in str(ei.value)
    assert side["n"] == 0


def test_r06_sandbox_allows_pure(tmp_path) -> None:
    calls = {"n": 0}

    def compute(x: int = 0) -> int:
        calls["n"] += 1
        return x + 1

    traj = open_trajectory(
        "demo",
        "r06-pure",
        db_path=str(tmp_path / "t.sqlite"),
        mode="sandbox",
    )
    tool = Tool(name="compute", fn=compute, effect_class=EffectClass.PURE)
    result = exec_tool(traj, 1, {"args": {"x": 1}}, tool, seq=2)
    assert result.result == 2
    assert calls["n"] == 1


def test_r06_sandbox_allows_read_only(tmp_path) -> None:
    calls = {"n": 0}

    def read_config(key: str = "") -> str:
        calls["n"] += 1
        return "value"

    traj = open_trajectory(
        "demo",
        "r06-readonly",
        db_path=str(tmp_path / "t.sqlite"),
        mode=RunMode.SANDBOX,
    )
    tool = Tool(name="read_config", fn=read_config, effect_class=EffectClass.READ_ONLY)
    result = exec_tool(traj, 1, {"args": {"key": "k"}}, tool, seq=2)
    assert result.result == "value"
    assert calls["n"] == 1


def test_r06_live_allows_non_idempotent(tmp_path) -> None:
    side = {"n": 0}

    def deploy(version: str = "1") -> dict:
        side["n"] += 1
        return {"deployed": version}

    traj = open_trajectory(
        "demo",
        "r06-live",
        db_path=str(tmp_path / "t.sqlite"),
        mode=RunMode.LIVE,
    )
    tool = Tool(name="deploy_server", fn=deploy, effect_class=EffectClass.NON_IDEMPOTENT_WRITE)
    result = exec_tool(traj, 1, {"args": {"version": "9"}}, tool, seq=2)
    assert result.result == {"deployed": "9"}
    assert side["n"] == 1
