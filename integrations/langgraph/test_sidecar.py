"""Unit tests for TrajIRToolGuard without LangGraph installed."""

from __future__ import annotations

import subprocess
import threading
from pathlib import Path
from types import SimpleNamespace

import pytest

from integrations.langgraph.sidecar import TrajIRToolGuard
from trajectory_ir.effects import EffectClass, OpenWorldOverrideRequired
from trajectory_ir.runtime.log import NodeLog

_ROOT = Path(__file__).resolve().parents[2]


def test_run_sealed_writes_decision_before_tool(tmp_path: Path) -> None:
    order: list[str] = []

    def boom(x: int) -> int:
        order.append("tool")
        return x + 1

    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="ut-1",
        work_dir=tmp_path,
        effect_hints={"boom": EffectClass.PURE},
    ) as guard:
        # Monkeypatch seal path observation via kinds after run
        out = guard.run_sealed("boom", {"x": 1}, boom)
        assert out == 2
        guard.finish()

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        kinds = [n["kind"] for n in log.list_nodes("ut-1", tenant_id="demo")]
    finally:
        log.close()
    assert kinds[0] == "PROJECT_CONTEXT"
    assert kinds[1] == "DECISION"
    assert "TOOL_CALL" in kinds
    assert kinds.index("DECISION") < kinds.index("TOOL_CALL")
    assert "tool" in order


def test_open_world_refuse_without_override(tmp_path: Path) -> None:
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="ut-ow",
        work_dir=tmp_path,
        effect_hints={"bash": EffectClass.READ_ONLY},
    ) as guard:
        with pytest.raises(OpenWorldOverrideRequired):
            guard.run_sealed("bash", {"cmd": "echo hi"}, lambda cmd: cmd)


def test_export_and_trajir_verify(tmp_path: Path) -> None:
    tir_path = tmp_path / "pack.tir"
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="ut-verify",
        work_dir=tmp_path / "run",
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:
        guard.run_sealed("echo", {"msg": "z"}, lambda msg: msg)
        guard.export_tir(tir_path)

    exe = _ROOT / "go" / "trajir.exe"
    if not exe.is_file():
        subprocess.check_call(
            ["go", "build", "-o", str(exe), "./cmd/trajir"],
            cwd=str(_ROOT / "go"),
        )
    proc = subprocess.run(
        [str(exe), "verify", str(tir_path)],
        check=False,
        capture_output=True,
        text=True,
    )
    assert proc.returncode == 0, proc.stderr


def test_second_turn_seals_a_new_step(tmp_path: Path) -> None:
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="ut-turns",
        work_dir=tmp_path,
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:
        guard.run_sealed("echo", {"msg": "a"}, lambda msg: msg)
        guard.run_sealed("echo", {"msg": "b"}, lambda msg: msg)
        guard.finish()

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("ut-turns", tenant_id="demo")
    finally:
        log.close()
    decisions = [r for r in rows if r["kind"] == "DECISION"]
    tools = [r for r in rows if r["kind"] == "TOOL_CALL"]
    assert [r["step_n"] for r in decisions] == [1, 2]
    assert [r["step_n"] for r in tools] == [1, 2]
    assert tools[0]["payload"]["args"] == {"msg": "a"}
    assert tools[1]["payload"]["args"] == {"msg": "b"}
    assert tools[0]["seq"] == 2
    assert tools[1]["seq"] == 2
    for row in rows:
        if row["kind"] != "TOOL_CALL":
            continue
        decision = next(d for d in decisions if d["step_n"] == row["step_n"])
        assert decision["seq"] < row["seq"]


def test_parallel_same_plan_gets_distinct_seq(tmp_path: Path) -> None:
    plan = [
        {"name": "echo", "args": {"msg": "a"}},
        {"name": "echo", "args": {"msg": "b"}},
    ]
    errors: list[BaseException] = []
    barrier = threading.Barrier(2)

    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="ut-par",
        work_dir=tmp_path,
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:

        def run(msg: str) -> None:
            try:
                barrier.wait(timeout=5)
                guard.run_sealed("echo", {"msg": msg}, lambda msg: msg, plan_calls=plan)
            except BaseException as exc:
                errors.append(exc)

        threads = [
            threading.Thread(target=run, args=("a",)),
            threading.Thread(target=run, args=("b",)),
        ]
        for thread in threads:
            thread.start()
        for thread in threads:
            thread.join(timeout=15)
            assert not thread.is_alive()
        assert errors == []
        guard.finish()

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("ut-par", tenant_id="demo")
    finally:
        log.close()
    decisions = [r for r in rows if r["kind"] == "DECISION"]
    tools = [r for r in rows if r["kind"] == "TOOL_CALL"]
    assert len(decisions) == 1
    assert {r["step_n"] for r in tools} == {1}
    assert sorted(r["seq"] for r in tools) == [2, 4]


def test_wrap_returns_none_when_tool_message_is_none(tmp_path: Path) -> None:
    request = SimpleNamespace(
        tool_call={"name": "echo", "args": {"msg": "x"}, "id": "call-1"},
        state={"messages": [SimpleNamespace(tool_calls=[{"name": "echo", "args": {"msg": "x"}}])]},
    )
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="ut-none",
        work_dir=tmp_path,
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:
        msg = guard.wrap_tool_call(request, lambda _request: None)
    assert msg is None


def test_wrap_returns_tool_message_when_body_skipped(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    def fake_exec(trajectory, step_n, call, tool, seq):
        return SimpleNamespace(result="from-resume")

    monkeypatch.setattr("integrations.langgraph.sidecar.exec_tool", fake_exec)
    request = SimpleNamespace(
        tool_call={"name": "echo", "args": {"msg": "x"}, "id": "call-9"},
        state={"messages": [SimpleNamespace(tool_calls=[{"name": "echo", "args": {"msg": "x"}}])]},
    )
    ran: list[str] = []

    def execute(_request: object) -> SimpleNamespace:
        ran.append("execute")
        return SimpleNamespace(content="nope")

    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="ut-skip",
        work_dir=tmp_path,
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:
        msg = guard.wrap_tool_call(request, execute)

    assert ran == []
    assert msg is not None
    assert getattr(msg, "content", None) == "from-resume"
    assert getattr(msg, "tool_call_id", None) == "call-9"
    assert getattr(msg, "type", None) == "tool"
