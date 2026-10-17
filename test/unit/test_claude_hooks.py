"""Hook processes block a bad pre-tool event and do not raise on a bad post-tool event."""

from __future__ import annotations

import json
from io import StringIO
from pathlib import Path

import pytest
from integrations.claude.config import tool_use_id
from integrations.claude.post_tool_use import main as post_main
from integrations.claude.post_tool_use_failure import main as failure_main
from integrations.claude.pre_tool_use import main as pre_main

from trajectory_ir.runtime.log import NodeLog


def test_pre_tool_rejects_bad_json(monkeypatch) -> None:
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert pre_main() == 2


def test_post_tool_ignores_bad_json(monkeypatch) -> None:
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert post_main() == 0


def test_pre_tool_seals_from_stdin(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-1")
    event = {
        "tool_name": "echo",
        "tool_use_id": "stdin-use",
        "tool_input": {"msg": "hi"},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    assert (tmp_path / "nodes.sqlite").is_file()


def test_tool_use_id_rejects_a_missing_id() -> None:
    with pytest.raises(ValueError, match="tool_use_id"):
        tool_use_id({"tool_name": "send_email"})


def test_pre_tool_rejects_a_missing_id(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-miss")
    event = {"tool_name": "send_email", "tool_input": {"to": "a"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    assert not (tmp_path / "nodes.sqlite").exists()


def test_failure_hook_records_abort_and_ignores_bad_json(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-fail")
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert failure_main() == 0

    seal = {"tool_name": "Bash", "tool_use_id": "fail-1", "tool_input": {"command": "false"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(seal)))
    assert pre_main() == 0
    failed = {"tool_use_id": "fail-1", "error": "exit 1"}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(failed)))
    assert failure_main() == 0

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("stdin-fail", tenant_id="demo")
    finally:
        log.close()
    abort = next(row for row in rows if row["kind"] == "ABORT")
    assert abort["payload"]["reason"] == "HOST_REPORTED_FAILURE"
    assert abort["payload"]["error"] == "exit 1"
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert "fail-1" not in state["pending"]
