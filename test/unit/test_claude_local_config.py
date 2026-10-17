"""The Claude installer merges local JSON and refuses to overwrite a broken file."""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path

_SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "write_claude_local_config.py"


def _load():
    spec = importlib.util.spec_from_file_location("write_claude_local_config", _SCRIPT)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _commands(settings: dict) -> list[str]:
    found: list[str] = []
    hooks = settings.get("hooks", {})
    for groups in hooks.values():
        for group in groups:
            for hook in group.get("hooks", []):
                found.append(str(hook.get("command") or ""))
    return found


def test_merge_keeps_permissions_and_other_mcp_servers(tmp_path: Path) -> None:
    module = _load()
    settings_path = tmp_path / ".claude" / "settings.local.json"
    settings_path.parent.mkdir(parents=True)
    settings_path.write_text(
        json.dumps({"permissions": {"allow": ["Bash"]}}) + "\n",
        encoding="utf-8",
    )
    mcp_path = tmp_path / ".mcp.json"
    mcp_path.write_text(
        json.dumps({"mcpServers": {"other": {"command": "keep-me"}}}) + "\n",
        encoding="utf-8",
    )
    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 0
    settings = json.loads(settings_path.read_text(encoding="utf-8"))
    assert settings["permissions"]["allow"] == ["Bash"]
    commands = _commands(settings)
    assert sum("pre_tool_use.py" in command for command in commands) == 1
    assert sum("post_tool_use.py" in command for command in commands) == 1
    assert sum("post_tool_use_failure.py" in command for command in commands) == 1

    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 0
    again = json.loads(settings_path.read_text(encoding="utf-8"))
    commands = _commands(again)
    assert sum("pre_tool_use.py" in command for command in commands) == 1
    assert sum("post_tool_use_failure.py" in command for command in commands) == 1
    mcp = json.loads(mcp_path.read_text(encoding="utf-8"))
    assert mcp["mcpServers"]["other"]["command"] == "keep-me"
    assert "trajectory-ir" in mcp["mcpServers"]


def test_invalid_json_is_left_unchanged(tmp_path: Path) -> None:
    module = _load()
    settings_path = tmp_path / ".claude" / "settings.local.json"
    settings_path.parent.mkdir(parents=True)
    settings_path.write_text("{", encoding="utf-8")
    mcp_path = tmp_path / ".mcp.json"
    mcp_path.write_text('{"mcpServers": {"other": {"command": "keep-me"}}}\n', encoding="utf-8")
    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 1
    assert settings_path.read_text(encoding="utf-8") == "{"
    assert "trajectory-ir" not in mcp_path.read_text(encoding="utf-8")

    settings_path.write_text("{}\n", encoding="utf-8")
    mcp_path.write_text("[", encoding="utf-8")
    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 1
    assert mcp_path.read_text(encoding="utf-8") == "["
    assert settings_path.read_text(encoding="utf-8") == "{}\n"
