"""Seal happens before record, and record does not call a real tool."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import time
from pathlib import Path

import pytest
from integrations.hosthook.session import (
    record_tool_failure,
    record_tool_result,
    seal_before_tool,
)
from integrations.otel.correlate import push_span, reset_span

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
    assert again["duplicate"] is True
    assert "replay" not in again
    saved = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert "replay" not in saved["pending"]["use-1"]
    assert "duplicate" not in saved["pending"]["use-1"]

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


def test_same_owner_bash_pending_is_a_replay(tmp_path: Path) -> None:
    first = seal_before_tool(
        tmp_path,
        "demo",
        "hook-bash-again",
        "Bash",
        {"command": "echo hi"},
        "use-again",
        owner="project",
    )
    assert first["effect_class"] == "NON_IDEMPOTENT_WRITE"
    again = seal_before_tool(
        tmp_path,
        "demo",
        "hook-bash-again",
        "Bash",
        {"command": "echo hi"},
        "use-again",
        owner="project",
    )
    assert again["replay"] is True
    other = seal_before_tool(
        tmp_path,
        "demo",
        "hook-bash-again",
        "Bash",
        {"command": "echo hi"},
        "use-again",
        owner="user",
    )
    assert other["duplicate"] is True
    assert "replay" not in other
    saved = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert saved["pending"]["use-again"]["owner"] == "project"
    assert "replay" not in saved["pending"]["use-again"]
    assert saved["pending"]["use-again"]["owner"] == "project"


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
    "client": "trajectory_ir.client.python.trajectory_client" in sys.modules,
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


def test_held_stale_lock_times_out_instead_of_spinning(tmp_path: Path, monkeypatch) -> None:
    """A lock another process still holds must sleep and time out.

    On Windows, unlinking a lock that is still open raises PermissionError.
    Continuing straight back into the acquire loop never reaches the deadline.
    """
    lock = tmp_path / "hook.lock"
    lock.write_text("held", encoding="utf-8")
    os.utime(lock, (0, 0))

    spins = {"slept": True}
    real_unlink = Path.unlink

    def unlink_held(self: Path, *args: object, **kwargs: object) -> None:
        if self.name != "hook.lock":
            return real_unlink(self, *args, **kwargs)
        if not spins["slept"]:
            raise AssertionError("lock retry did not sleep")
        spins["slept"] = False
        raise PermissionError("sharing violation")

    clock = {"t": 1_000.0}
    monkeypatch.setattr(Path, "unlink", unlink_held)
    monkeypatch.setattr(time, "time", lambda: clock["t"])

    def sleep(seconds: float) -> None:
        spins["slept"] = True
        clock["t"] += seconds

    monkeypatch.setattr(time, "sleep", sleep)

    with pytest.raises(TimeoutError, match="hook lock"):
        seal_before_tool(tmp_path, "demo", "hook-lock", "echo", {"msg": "a"}, "use-lock")
    assert not (tmp_path / "nodes.sqlite").exists()


def test_removable_stale_lock_is_replaced(tmp_path: Path) -> None:
    lock = tmp_path / "hook.lock"
    lock.write_text("stale", encoding="utf-8")
    stale = time.time() - 60
    os.utime(lock, (stale, stale))
    sealed = seal_before_tool(tmp_path, "demo", "hook-steal", "echo", {"msg": "a"}, "use-steal")
    assert sealed["sealed"] is True
    assert (tmp_path / "nodes.sqlite").is_file()


class _Span:
    def __init__(self) -> None:
        self.attrs: dict[str, object] = {}

    def set_attribute(self, key: str, value: object) -> None:
        self.attrs[key] = value


def test_seal_labels_the_decision_it_just_wrote(tmp_path: Path, monkeypatch) -> None:
    monkeypatch.setenv("TRAJIR_OTEL_CORRELATE", "1")
    calls = {"n": 0}
    real_list = NodeLog.list_nodes

    def counting(self: NodeLog, *args: object, **kwargs: object) -> list[dict]:
        calls["n"] += 1
        return real_list(self, *args, **kwargs)

    monkeypatch.setattr(NodeLog, "list_nodes", counting)
    span = _Span()
    token = push_span(span)
    try:
        seal_before_tool(tmp_path, "demo", "hook-otel", "echo", {"msg": "hi"}, "use-otel")
    finally:
        reset_span(token)

    assert calls["n"] == 0
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = real_list(log, "hook-otel", tenant_id="demo")
    finally:
        log.close()
    decision = next(row for row in rows if row["kind"] == "DECISION")
    assert span.attrs["trajir.node_id"] == decision["id"]
    assert span.attrs["trajir.content_hash"]
    assert span.attrs["trajir.step_n"] == 1
