"""Seal happens before record, and record does not call a real tool."""

from __future__ import annotations

from pathlib import Path

from integrations.hosthook.session import record_tool_result, seal_before_tool
from trajectory_ir.runtime.log import NodeLog


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
        annotations={"readOnlyHint": True, "destructiveHint": False},
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
        annotations={"readOnlyHint": True, "destructiveHint": False},
    )
    assert again["step_n"] == 1

    recorded = record_tool_result(tmp_path, "demo", "hook-1", "use-1", "hello")
    assert recorded["recorded"] is True
    assert calls["n"] == 0
    second = record_tool_result(tmp_path, "demo", "hook-1", "use-1", "hello")
    assert second["step_n"] == 1

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("hook-1", tenant_id="demo")
    finally:
        log.close()
    kinds = [row["kind"] for row in rows]
    assert kinds.index("DECISION") < kinds.index("TOOL_CALL")
    assert kinds.index("TOOL_CALL") < kinds.index("TOOL_RESULT")
    tool = next(row for row in rows if row["kind"] == "TOOL_CALL")
    assert tool["payload"]["idempotency_key"]
    assert "idempotency_key" not in tool["payload"]["args"]


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
