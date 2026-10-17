"""Write the project-local Claude hook and MCP config. Does not touch the user profile.

Existing settings.local.json and .mcp.json are merged. A file that is not a
JSON object is left unchanged and the command exits 1.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[1]
_HOOK_EVENTS = (
    ("PreToolUse", "pre_tool_use.py"),
    ("PostToolUse", "post_tool_use.py"),
    ("PostToolUseFailure", "post_tool_use_failure.py"),
)


def _command(py: str, script: Path) -> str:
    return f'"{py}" "{script}"'


def _read_object(path: Path) -> dict | None:
    if not path.is_file():
        return None
    try:
        loaded = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValueError(f"{path.name} is not valid JSON ({exc})") from exc
    if not isinstance(loaded, dict):
        raise ValueError(f"{path.name} must be a JSON object")
    return loaded


def _hook_commands(groups: object) -> list[str]:
    found: list[str] = []
    if not isinstance(groups, list):
        return found
    for group in groups:
        if not isinstance(group, dict):
            continue
        hooks = group.get("hooks")
        if not isinstance(hooks, list):
            continue
        for hook in hooks:
            if isinstance(hook, dict):
                found.append(str(hook.get("command") or ""))
    return found


def _with_hook(settings: dict, event: str, script: Path, py: str) -> dict:
    hooks = settings.get("hooks")
    hooks = dict(hooks) if isinstance(hooks, dict) else {}
    groups = hooks.get(event)
    groups = list(groups) if isinstance(groups, list) else []
    if any(script.name in command for command in _hook_commands(groups)):
        return {**settings, "hooks": {**hooks, event: groups}}
    added = {
        "matcher": "*",
        "hooks": [{"type": "command", "command": _command(py, script)}],
    }
    return {**settings, "hooks": {**hooks, event: [*groups, added]}}


def _merged_settings(existing: dict | None, root: Path, py: str) -> dict:
    settings = dict(existing or {})
    for event, name in _HOOK_EVENTS:
        script = root / "integrations" / "claude" / name
        settings = _with_hook(settings, event, script, py)
    return settings


def _merged_mcp(existing: dict | None, root: Path, mcp_bin: Path) -> dict:
    base = dict(existing or {})
    servers = base.get("mcpServers")
    servers = dict(servers) if isinstance(servers, dict) else {}
    servers["trajectory-ir"] = {
        "command": str(mcp_bin),
        "args": [],
        "env": {"TRAJIR_MCP_ROOT": str(root)},
    }
    return {**base, "mcpServers": servers}


def _mcp_binary(root: Path) -> Path:
    exe = root / "go" / "trajir-mcp.exe"
    if exe.is_file():
        return exe
    plain = root / "go" / "trajir-mcp"
    return plain if plain.is_file() else exe


def _write_json(path: Path, payload: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


def write_local_config(root: Path, py: str, mcp_bin: Path) -> int:
    """Merge TrajIR hooks and the MCP server into project-local JSON files."""
    root = Path(root)
    settings_path = root / ".claude" / "settings.local.json"
    mcp_path = root / ".mcp.json"
    try:
        settings = _read_object(settings_path)
        mcp_cfg = _read_object(mcp_path)
    except ValueError as exc:
        print(f"trajir claude config: {exc}", file=sys.stderr)
        return 1

    work = root / ".local-evidence" / "claude-agent"
    work.mkdir(parents=True, exist_ok=True)
    ident = {
        "work_dir": str(work),
        "tenant_id": "local",
        "trajectory_id": "claude-one-step",
    }
    ident_path = root / ".local-evidence" / "claude-one-step.json"
    _write_json(ident_path, ident)
    _write_json(settings_path, _merged_settings(settings, root, py))
    _write_json(mcp_path, _merged_mcp(mcp_cfg, root, mcp_bin))
    print(settings_path)
    print(mcp_path)
    print(ident_path)
    return 0


def main() -> int:
    return write_local_config(_ROOT, sys.executable, _mcp_binary(_ROOT))


if __name__ == "__main__":
    raise SystemExit(main())
