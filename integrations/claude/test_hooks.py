"""Hook processes block a bad pre-tool event and do not raise on a bad post-tool event."""

from __future__ import annotations

import json
from io import StringIO

from integrations.claude.post_tool_use import main as post_main
from integrations.claude.pre_tool_use import main as pre_main


def test_pre_tool_rejects_bad_json(monkeypatch) -> None:
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert pre_main() == 2


def test_post_tool_ignores_bad_json(monkeypatch) -> None:
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert post_main() == 0


def test_pre_tool_seals_from_stdin(monkeypatch, tmp_path) -> None:
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
