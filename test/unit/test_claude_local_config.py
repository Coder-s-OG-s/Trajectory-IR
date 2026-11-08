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
    assert not (tmp_path / ".cursor").exists()


def test_target_dir_keeps_hooks_in_the_install(tmp_path: Path) -> None:
    module = _load()
    install = tmp_path / "install"
    target = tmp_path / "project"
    mcp_bin = install / "go" / "trajir-mcp.exe"
    assert module.write_local_config(install, sys.executable, mcp_bin, target=target) == 0
    assert not (install / ".claude").exists()
    assert not (install / ".mcp.json").exists()
    settings = json.loads((target / ".claude" / "settings.local.json").read_text(encoding="utf-8"))
    commands = _commands(settings)
    assert any("launch_hook.py" in command and "pre_tool_use.py" in command for command in commands)
    assert any(str(install) in command for command in commands)
    ident = json.loads(
        (target / ".local-evidence" / "claude-one-step.json").read_text(encoding="utf-8")
    )
    assert ident["work_dir"].startswith(str(target))
    assert Path(ident["work_dir"]).is_absolute()
    ignore = (target / ".gitignore").read_text(encoding="utf-8").splitlines()
    for line in (
        ".local-evidence/",
        ".claude/settings.local.json",
        ".mcp.json",
        ".cursor/mcp.json",
        ".vscode/mcp.json",
        ".gemini/settings.json",
        ".codex/config.toml",
        ".trajir/*.py",
    ):
        assert line in ignore
    launcher = next(
        command
        for command in commands
        if "launch_hook.py" in command and "pre_tool_use.py" in command
    )
    assert launcher.endswith('"project"')
    mcp = json.loads((target / ".mcp.json").read_text(encoding="utf-8"))
    assert mcp["mcpServers"]["trajectory-ir"]["env"]["TRAJIR_MCP_ROOT"] == str(target)
    assert mcp["mcpServers"]["trajectory-ir"]["command"] == str(mcp_bin)
    cursor = json.loads((target / ".cursor" / "mcp.json").read_text(encoding="utf-8"))
    assert "trajectory-ir" in cursor["mcpServers"]
    vscode = json.loads((target / ".vscode" / "mcp.json").read_text(encoding="utf-8"))
    assert vscode["servers"]["trajectory-ir"]["type"] == "stdio"
    gemini = json.loads((target / ".gemini" / "settings.json").read_text(encoding="utf-8"))
    assert "trajectory-ir" in gemini["mcpServers"]
    codex = (target / ".codex" / "config.toml").read_text(encoding="utf-8")
    assert codex.count("\n[mcp_servers.trajectory-ir]\n") == 1
    lang_path = target / ".trajir" / "langgraph_attach.py"
    adk_path = target / ".trajir" / "adk_attach.py"
    langgraph = lang_path.read_text(encoding="utf-8")
    adk = adk_path.read_text(encoding="utf-8")
    assert langgraph.index("def attach") < langgraph.index("from langgraph")
    assert "import dbos" not in langgraph.lower()
    assert "from dbos" not in langgraph.lower()
    assert adk.index("def callbacks") < adk.index("from integrations.adk")
    assert "import google" not in adk
    loaded: dict[str, object] = {}
    for name, path in (("lg_attach_under_test", lang_path), ("adk_attach_under_test", adk_path)):
        spec = importlib.util.spec_from_file_location(name, path)
        assert spec is not None and spec.loader is not None
        snippet = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(snippet)
        loaded[name] = snippet
    assert callable(loaded["lg_attach_under_test"].attach)
    assert callable(loaded["adk_attach_under_test"].callbacks)
    assert module.write_local_config(install, sys.executable, mcp_bin, target=target) == 0
    again = (target / ".codex" / "config.toml").read_text(encoding="utf-8")
    assert again.count("\n[mcp_servers.trajectory-ir]\n") == 1


def test_invalid_cursor_json_writes_nothing(tmp_path: Path) -> None:
    module = _load()
    cursor = tmp_path / ".cursor" / "mcp.json"
    cursor.parent.mkdir(parents=True)
    cursor.write_text("[", encoding="utf-8")
    settings = tmp_path / ".claude" / "settings.local.json"
    settings.parent.mkdir(parents=True)
    settings.write_text("{}\n", encoding="utf-8")
    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 1
    assert settings.read_text(encoding="utf-8") == "{}\n"
    assert cursor.read_text(encoding="utf-8") == "["
    assert not (tmp_path / ".mcp.json").exists()


def test_relative_target_is_stored_absolute(monkeypatch, tmp_path: Path) -> None:
    module = _load()
    project = tmp_path / "project"
    project.mkdir()
    monkeypatch.chdir(tmp_path)
    assert module.main(["--skip-console", "--target", "project"]) == 0
    ident = json.loads(
        (project / ".local-evidence" / "claude-one-step.json").read_text(encoding="utf-8")
    )
    assert Path(ident["work_dir"]).is_absolute()
    assert Path(ident["work_dir"]).is_relative_to(project.resolve())
    mcp = json.loads((project / ".mcp.json").read_text(encoding="utf-8"))
    command = mcp["mcpServers"]["trajectory-ir"]["command"]
    assert Path(command).is_absolute()


def test_user_hook_that_only_names_the_script_is_kept(tmp_path: Path) -> None:
    module = _load()
    settings_path = tmp_path / ".claude" / "settings.local.json"
    settings_path.parent.mkdir(parents=True)
    user_cmd = "python pre_tool_use.py"
    settings_path.write_text(
        json.dumps(
            {
                "hooks": {
                    "PreToolUse": [
                        {"matcher": "*", "hooks": [{"type": "command", "command": user_cmd}]}
                    ]
                }
            }
        )
        + "\n",
        encoding="utf-8",
    )
    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 0
    commands = _commands(json.loads(settings_path.read_text(encoding="utf-8")))
    assert user_cmd in commands
    assert sum("launch_hook.py" in command for command in commands) == 3
    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 0
    again = _commands(json.loads(settings_path.read_text(encoding="utf-8")))
    assert again.count(user_cmd) == 1
    assert sum("launch_hook.py" in command for command in again) == 3


def test_forward_slash_launcher_is_replaced(tmp_path: Path) -> None:
    module = _load()
    settings_path = tmp_path / ".claude" / "settings.local.json"
    settings_path.parent.mkdir(parents=True)
    launcher = tmp_path / "integrations" / "attach" / "launch_hook.py"
    forward = str(launcher).replace("\\", "/")
    user_cmd = "echo keep-me"
    settings_path.write_text(
        json.dumps(
            {
                "hooks": {
                    "PreToolUse": [
                        {
                            "matcher": "*",
                            "hooks": [
                                {"type": "command", "command": user_cmd},
                                {
                                    "type": "command",
                                    "command": f'python "{forward}" id script pre project',
                                },
                            ],
                        }
                    ]
                }
            }
        )
        + "\n",
        encoding="utf-8",
    )
    assert module.write_local_config(tmp_path, sys.executable, tmp_path / "trajir-mcp") == 0
    commands = _commands(json.loads(settings_path.read_text(encoding="utf-8")))
    assert commands.count(user_cmd) == 1
    assert sum("launch_hook.py" in command for command in commands) == 3


def test_gitignore_keeps_an_existing_local_evidence_line(tmp_path: Path) -> None:
    module = _load()
    target = tmp_path / "project"
    target.mkdir()
    (target / ".gitignore").write_text("keep-me\n.local-evidence\n", encoding="utf-8")
    install = tmp_path / "install"
    assert module.write_local_config(install, sys.executable, install / "mcp", target=target) == 0
    lines = (target / ".gitignore").read_text(encoding="utf-8").splitlines()
    assert lines[0] == "keep-me"
    assert lines[1] == ".local-evidence"
    assert ".local-evidence/" not in lines
    assert ".mcp.json" in lines
    assert ".trajir/*.py" in lines


def test_console_down_exits_1(monkeypatch, tmp_path: Path, capsys) -> None:
    module = _load()

    def down(*_args: object, **_kwargs: object) -> object:
        return type("Launch", (), {"status": "down"})()

    monkeypatch.setattr(module, "start_console_if_free", down)
    assert module.main(["--target", str(tmp_path)]) == 1
    assert "console did not stay up" in capsys.readouterr().err


def test_console_start_uses_the_target_data_dir(monkeypatch, tmp_path: Path) -> None:
    module = _load()
    seen: dict[str, object] = {}

    def started(*_args: object, **kwargs: object) -> object:
        seen["data"] = kwargs.get("data_dir")
        return type("Launch", (), {"status": "started"})()

    monkeypatch.setattr(module, "start_console_if_free", started)
    assert module.main(["--target", str(tmp_path)]) == 0
    expected = (tmp_path / ".local-evidence" / "console-data").resolve()
    assert Path(str(seen["data"])) == expected


def test_install_scripts_resolve_target_before_changing_directory() -> None:
    root = Path(__file__).resolve().parents[2]
    shell = (root / "scripts" / "install_claude.sh").read_text(encoding="utf-8")
    assert shell.index('TARGET="$(cd "$2" && pwd)"') < shell.index('cd "$ROOT"')
    assert "TRAJIR_PYTHON" in shell
    assert "command -v python3" in shell
    assert ".venv/Scripts/python.exe" in shell
    assert shell.index(".venv/Scripts/python.exe") < shell.index("command -v python3")
    assert shell.index("TRAJIR_PYTHON") < shell.index(
        '"$PY" "$ROOT/scripts/write_claude_local_config.py"'
    )
    powershell = (root / "scripts" / "install_claude.ps1").read_text(encoding="utf-8")
    assert powershell.index("Resolve-Path -LiteralPath $Target") < powershell.index(
        "Set-Location $root"
    )


def test_skip_console_does_not_start(monkeypatch, tmp_path: Path) -> None:
    module = _load()

    def boom(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("console start was called")

    monkeypatch.setattr(module, "start_console_if_free", boom)
    assert module.main(["--skip-console", "--target", str(tmp_path)]) == 0
    assert (tmp_path / ".mcp.json").is_file()
