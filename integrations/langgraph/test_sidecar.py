"""Unit tests for TrajIRToolGuard without LangGraph installed."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

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
