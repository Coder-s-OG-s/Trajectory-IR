"""Integration tests for the TURNING_POINT evidence sidecar (local shipable cut)."""

from __future__ import annotations

import json
from pathlib import Path
from types import SimpleNamespace

import pytest

from integrations.langgraph.sidecar import TrajIRToolGuard
from integrations.langgraph.verify_pack import (
    ensure_trajir_built,
    run_verify,
    verify_and_note_audit,
)
from trajectory_ir.console_emit import FileSink
from trajectory_ir.effects import EffectClass, OpenWorldOverrideRequired
from trajectory_ir.runtime.log import NodeLog

_ROOT = Path(__file__).resolve().parents[2]


@pytest.fixture(scope="module")
def trajir_exe() -> Path:
    return ensure_trajir_built(_ROOT)


@pytest.mark.integration
def test_multi_tool_export_verify_ok_and_flip_fail(tmp_path: Path, trajir_exe: Path) -> None:
    tir = tmp_path / "pack.tir"
    plan = [
        {"name": "echo", "args": {"msg": "a"}},
        {"name": "ship", "args": {"svc": "api"}},
    ]
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="it-multi",
        work_dir=tmp_path / "run",
        effect_hints={
            "echo": EffectClass.PURE,
            "ship": EffectClass.NON_IDEMPOTENT_WRITE,
        },
    ) as guard:
        guard.run_sealed("echo", plan[0]["args"], lambda msg: msg, plan_calls=plan)
        guard.run_sealed("ship", plan[1]["args"], lambda svc: {"ok": svc}, plan_calls=plan)
        guard.export_tir(tir)

    log = NodeLog(str(tmp_path / "run" / "nodes.sqlite"))
    try:
        kinds = [n["kind"] for n in log.list_nodes("it-multi", tenant_id="demo")]
    finally:
        log.close()
    assert kinds.index("DECISION") < kinds.index("TOOL_CALL")

    result = run_verify(tir, root=_ROOT)
    assert result["ok"] is True, result

    bad = tmp_path / "flipped.tir"
    data = bytearray(tir.read_bytes())
    data[len(data) // 2] ^= 0xFF
    bad.write_bytes(data)
    flipped = run_verify(bad, root=_ROOT)
    assert flipped["ok"] is False


@pytest.mark.integration
def test_open_world_refuse(tmp_path: Path) -> None:
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="it-ow",
        work_dir=tmp_path,
        effect_hints={"bash": EffectClass.READ_ONLY},
    ) as guard:
        with pytest.raises(OpenWorldOverrideRequired):
            guard.run_sealed("bash", {"cmd": "echo hi"}, lambda cmd: cmd)


@pytest.mark.integration
def test_console_file_sink_roundtrip(tmp_path: Path, trajir_exe: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    console_root = tmp_path / "console"
    monkeypatch.setenv("TRAJIR_CONSOLE_SINK", "file")
    monkeypatch.setenv("TRAJIR_CONSOLE_DATA", str(console_root))

    tir = tmp_path / "pack.tir"
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="it-console",
        work_dir=tmp_path / "run",
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:
        guard.run_sealed("echo", {"msg": "console"}, lambda msg: msg)
        guard.export_tir(tir)

    result = verify_and_note_audit(
        tir,
        trajectory_id="it-console",
        tenant_id="demo",
        root=_ROOT,
    )
    assert result["ok"] is True

    ndjson = console_root / "trajectories" / "it-console.ndjson"
    assert ndjson.is_file()
    events = [json.loads(line) for line in ndjson.read_text(encoding="utf-8").splitlines() if line]
    kinds = {e["kind"] for e in events}
    assert "seal.created" in kinds
    assert "export.completed" in kinds
    assert "audit.completed" in kinds

    tool_events = [
        e
        for e in events
        if e["kind"] == "node.appended" and e["payload"].get("kind") == "TOOL_CALL"
    ]
    assert tool_events
    payload = tool_events[0]["payload"]
    assert payload["tool"] == "echo"
    assert payload["effect_class"] == "PURE"
    assert payload.get("idempotency_key")

    audits = [e for e in events if e["kind"] == "audit.completed"]
    assert audits[-1]["payload"]["ok"] is True


@pytest.mark.integration
def test_wrap_tool_call_seals_before_execute(tmp_path: Path) -> None:
    order: list[str] = []

    def execute(request: object) -> SimpleNamespace:
        order.append("execute")
        return SimpleNamespace(content="done")

    request = SimpleNamespace(
        tool_call={"name": "echo", "args": {"msg": "x"}},
        state={"messages": [SimpleNamespace(tool_calls=[{"name": "echo", "args": {"msg": "x"}}])]},
    )

    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="it-wrap",
        work_dir=tmp_path,
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:
        # Observe seal by checking NodeLog after wrap; execute must run after DECISION.
        msg = guard.wrap_tool_call(request, execute)
        assert getattr(msg, "content", None) == "done"
        guard.finish()

    assert order == ["execute"]
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("it-wrap", tenant_id="demo")
    finally:
        log.close()
    kinds = [r["kind"] for r in rows]
    assert kinds.index("DECISION") < kinds.index("TOOL_CALL")
    tool_row = next(r for r in rows if r["kind"] == "TOOL_CALL")
    assert "idempotency_key" in tool_row["payload"]
    # Idempotency key must never be injected into tool kwargs (args stay caller args).
    assert tool_row["payload"]["args"] == {"msg": "x"}
    assert "idempotency_key" not in tool_row["payload"]["args"]


@pytest.mark.integration
def test_file_sink_explicit_note_audit(tmp_path: Path, trajir_exe: Path) -> None:
    """audit.completed works with an explicit sink (not only from_env)."""
    tir = tmp_path / "pack.tir"
    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="it-audit",
        work_dir=tmp_path / "run",
        effect_hints={"echo": EffectClass.PURE},
    ) as guard:
        guard.run_sealed("echo", {"msg": "a"}, lambda msg: msg)
        guard.export_tir(tir)

    sink = FileSink(str(tmp_path / "console"))
    result = verify_and_note_audit(
        tir,
        trajectory_id="it-audit",
        tenant_id="demo",
        sink=sink,
        root=_ROOT,
    )
    assert result["ok"] is True
    path = tmp_path / "console" / "trajectories" / "it-audit.ndjson"
    lines = path.read_text(encoding="utf-8").strip().splitlines()
    last = json.loads(lines[-1])
    assert last["kind"] == "audit.completed"
    assert last["payload"]["ok"] is True
