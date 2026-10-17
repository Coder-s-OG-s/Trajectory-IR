"""Seal happens before record, and record does not call a real tool."""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

from integrations.hosthook.session import (
    record_tool_failure,
    record_tool_result,
    seal_before_tool,
)

from trajectory_ir.runtime.log import NodeLog

_ROOT = Path(__file__).resolve().parents[2]
_READ_ONLY = {"readOnlyHint": True, "destructiveHint": False}


def _rows(work_dir: Path, trajectory_id: str) -> list[dict]:
    log = NodeLog(str(work_dir / "nodes.sqlite"))
    try:
        return log.list_nodes(trajectory_id, tenant_id="demo")
    finally:
        log.close()


def test_seal_then_record_keeps_decision_first(tmp_path: Path) -> None:
    calls = {"n": 0}

    def real_tool(msg: str) -> str:
        calls["n"] += 1
        return msg

    sealed = seal_before_tool(
        tmp_path,
        "demo",
        "hook-1",
        "echo",
        {"msg": "hello"},
        "use-1",
        annotations=_READ_ONLY,
    )
    assert sealed["step_n"] == 1
    assert sealed["effect_class"] == "READ_ONLY"
    assert calls["n"] == 0

    again = seal_before_tool(
        tmp_path,
        "demo",
        "hook-1",
        "echo",
        {"msg": "hello"},
        "use-1",
        annotations=_READ_ONLY,
    )
    assert again["step_n"] == 1

    recorded = record_tool_result(tmp_path, "demo", "hook-1", "use-1", "hello")
    assert recorded["recorded"] is True
    assert calls["n"] == 0
    second = record_tool_result(tmp_path, "demo", "hook-1", "use-1", "hello")
    assert second["step_n"] == 1

    rows = _rows(tmp_path, "hook-1")
    kinds = [row["kind"] for row in rows]
    assert kinds.index("DECISION") < kinds.index("TOOL_CALL")
    assert kinds.index("TOOL_CALL") < kinds.index("TOOL_RESULT")
    tool = next(row for row in rows if row["kind"] == "TOOL_CALL")
    assert tool["payload"]["idempotency_key"]
    assert "idempotency_key" not in tool["payload"]["args"]
    assert calls["n"] == 0


def test_bash_fails_closed(tmp_path: Path) -> None:
    sealed = seal_before_tool(
        tmp_path,
        "demo",
        "hook-bash",
        "Bash",
        {"command": "echo hi"},
        "use-bash",
        annotations={"readOnlyHint": True},
    )
    assert sealed["effect_class"] == "NON_IDEMPOTENT_WRITE"


def test_second_tool_is_the_next_step(tmp_path: Path) -> None:
    seal_before_tool(tmp_path, "demo", "hook-2", "echo", {"msg": "a"}, "a")
    record_tool_result(tmp_path, "demo", "hook-2", "a", "a")
    nxt = seal_before_tool(tmp_path, "demo", "hook-2", "echo", {"msg": "b"}, "b")
    assert nxt["step_n"] == 2


def test_failed_read_only_tool_records_abort_once(tmp_path: Path) -> None:
    seal_before_tool(
        tmp_path,
        "demo",
        "hook-fail",
        "echo",
        {"msg": "x"},
        "use-fail",
        annotations=_READ_ONLY,
    )
    failed = record_tool_failure(tmp_path, "demo", "hook-fail", "use-fail", "boom")
    assert failed["failed"] is True
    assert failed["recorded"] is True
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert "use-fail" not in state["pending"]
    assert state["done"]["use-fail"]["failed"] is True

    rows = _rows(tmp_path, "hook-fail")
    kinds = [row["kind"] for row in rows]
    assert kinds.index("DECISION") < kinds.index("TOOL_CALL")
    assert kinds.index("TOOL_CALL") < kinds.index("ABORT")
    assert "COMMIT_STEP" in kinds
    assert "TOOL_RESULT" not in kinds
    abort = next(row for row in rows if row["kind"] == "ABORT")
    assert abort["payload"]["reason"] == "HOST_REPORTED_FAILURE"
    assert abort["payload"]["error"] == "boom"

    again = record_tool_failure(tmp_path, "demo", "hook-fail", "use-fail", "boom")
    assert again["step_n"] == failed["step_n"]
    rows_again = _rows(tmp_path, "hook-fail")
    assert [row["kind"] for row in rows_again].count("ABORT") == 1


def test_failed_gated_tool_does_not_write_a_second_abort(tmp_path: Path) -> None:
    sealed = seal_before_tool(
        tmp_path,
        "demo",
        "hook-gate",
        "Bash",
        {"command": "false"},
        "use-gate",
    )
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        log.append(
            "TOOL_CALL",
            sealed["step_n"],
            {"tool": "Bash", "args": {"command": "false"}, "idempotency_key": "taken"},
            "hook-gate",
            "demo",
            seq=2,
        )
        log.append(
            "ABORT",
            sealed["step_n"],
            {"reason": "BLOCKED_NEEDS_GATE", "tool": "Bash"},
            "hook-gate",
            "demo",
            seq=3,
        )
    finally:
        log.close()

    done = record_tool_failure(tmp_path, "demo", "hook-gate", "use-gate", "host failed")
    assert done["already_claimed"] is True
    rows = _rows(tmp_path, "hook-gate")
    aborts = [row for row in rows if row["kind"] == "ABORT"]
    assert len(aborts) == 1
    assert aborts[0]["payload"]["reason"] == "BLOCKED_NEEDS_GATE"
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert "use-gate" not in state["pending"]


def test_session_import_does_not_boot_dbos(tmp_path: Path) -> None:
    env = os.environ.copy()
    env["PYTHONPATH"] = os.pathsep.join([str(_ROOT), str(_ROOT / "pkg"), env.get("PYTHONPATH", "")])
    env.pop("TRAJIR_OTEL_CORRELATE", None)
    code = """
import json, sys, time
from integrations.hosthook.session import seal_before_tool
started = time.perf_counter()
seal_before_tool(sys.argv[1], "demo", "cold", "echo", {"msg": "hi"}, "use-cold")
print(json.dumps({
    "dbos": "dbos" in sys.modules,
    "client": "client.python.trajectory_client" in sys.modules,
    "elapsed": time.perf_counter() - started,
}))
"""
    proc = subprocess.run(
        [sys.executable, "-c", code, str(tmp_path)],
        cwd=_ROOT,
        env=env,
        capture_output=True,
        text=True,
        check=False,
        timeout=30,
    )
    assert proc.returncode == 0, proc.stderr
    loaded = json.loads(proc.stdout)
    assert loaded["dbos"] is False
    assert loaded["client"] is False
    assert loaded["elapsed"] < 1.0
