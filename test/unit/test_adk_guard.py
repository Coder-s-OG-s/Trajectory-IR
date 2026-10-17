"""ADK callbacks seal first and do not call the tool."""

from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace

import pytest
from integrations.adk.guard import AdkToolGuard

from trajectory_ir.runtime.log import NodeLog


def test_callbacks_do_not_run_the_tool(tmp_path: Path) -> None:
    ran = {"n": 0}

    def real_tool() -> None:
        ran["n"] += 1

    guard = AdkToolGuard("demo", "adk-1", tmp_path)
    tool = SimpleNamespace(name="echo", id="call-9")
    assert guard.before_tool_callback(tool, {"msg": "hi"}, None) is None
    assert ran["n"] == 0
    assert guard.after_tool_callback(tool, {"msg": "hi"}, None, {"msg": "hi"}) is None
    assert ran["n"] == 0
    real_tool()
    assert ran["n"] == 1

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        kinds = [row["kind"] for row in log.list_nodes("adk-1", tenant_id="demo")]
    finally:
        log.close()
    assert kinds.index("DECISION") < kinds.index("TOOL_RESULT")


def test_missing_call_id_writes_no_decision(tmp_path: Path) -> None:
    guard = AdkToolGuard("demo", "adk-miss", tmp_path)
    tool = SimpleNamespace(name="send_email")
    with pytest.raises(ValueError, match="tool call id"):
        guard.before_tool_callback(tool, {"to": "a"}, None)
    with pytest.raises(ValueError, match="tool call id"):
        guard.before_tool_callback(tool, {"to": "b"}, None)
    assert not (tmp_path / "nodes.sqlite").exists()
