"""Write project-local host config. Does not edit the user profile.

Existing JSON object files are merged. A file that is not a JSON object is
left unchanged and the command returns 1 before any write. Codex TOML is
appended when the trajectory-ir table is absent. A bad TOML file does not
fail the install.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

_HOOK_EVENTS = (
    ("PreToolUse", "pre_tool_use.py", "pre"),
    ("PostToolUse", "post_tool_use.py", "post"),
    ("PostToolUseFailure", "post_tool_use_failure.py", "post"),
)

_JSON_RELATIVE = {
    "settings": Path(".claude") / "settings.local.json",
    "mcp": Path(".mcp.json"),
    "cursor": Path(".cursor") / "mcp.json",
    "vscode": Path(".vscode") / "mcp.json",
    "gemini": Path(".gemini") / "settings.json",
}


def _command(py: str, launcher: Path, identity: Path, script: Path, kind: str) -> str:
    # The last word is the hook owner. A second registration must use another word.
    return f'"{py}" "{launcher}" "{identity}" "{script}" "{kind}" "project"'


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


def _write_json(path: Path, payload: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


def _with_hook(settings: dict, event: str, command: str, match: str) -> dict:
    """Refresh our launcher command. Leave a user hook that does not use it."""
    hooks = settings.get("hooks")
    hooks = dict(hooks) if isinstance(hooks, dict) else {}
    groups = hooks.get(event)
    groups = list(groups) if isinstance(groups, list) else []
    replaced = False
    new_groups: list[object] = []
    for group in groups:
        if not isinstance(group, dict):
            new_groups.append(group)
            continue
        hook_list = group.get("hooks")
        if not isinstance(hook_list, list):
            new_groups.append(dict(group))
            continue
        new_hooks: list[object] = []
        for hook in hook_list:
            text = str(hook.get("command") or "") if isinstance(hook, dict) else ""
            if isinstance(hook, dict) and match and match in text:
                new_hooks.append({**hook, "command": command})
                replaced = True
            else:
                new_hooks.append(hook)
        new_groups.append({**group, "hooks": new_hooks})
    if not replaced:
        new_groups.append({"matcher": "*", "hooks": [{"type": "command", "command": command}]})
    return {**settings, "hooks": {**hooks, event: new_groups}}


def _merged_settings(
    existing: dict | None,
    install: Path,
    dest: Path,
    py: str,
    identity: Path,
) -> dict:
    settings = dict(existing or {})
    launcher = install / "integrations" / "attach" / "launch_hook.py"
    match = str(launcher)
    for event, name, kind in _HOOK_EVENTS:
        script = install / "integrations" / "claude" / name
        command = _command(py, launcher, identity, script, kind)
        settings = _with_hook(settings, event, command, match)
    return settings


def _server(mcp_bin: Path, dest: Path) -> dict:
    return {
        "command": str(mcp_bin),
        "args": [],
        "env": {"TRAJIR_MCP_ROOT": str(dest)},
    }


def _merged_named(existing: dict | None, key: str, entry: dict) -> dict:
    base = dict(existing or {})
    servers = base.get(key)
    servers = dict(servers) if isinstance(servers, dict) else {}
    servers["trajectory-ir"] = entry
    return {**base, key: servers}


def _toml_string(value: str) -> str:
    escaped = value.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


def _codex_block(mcp_bin: Path, dest: Path) -> str:
    return (
        "\n[mcp_servers.trajectory-ir]\n"
        f"command = {_toml_string(str(mcp_bin))}\n"
        "args = []\n"
        "\n[mcp_servers.trajectory-ir.env]\n"
        f"TRAJIR_MCP_ROOT = {_toml_string(str(dest))}\n"
    )


def _write_codex(dest: Path, mcp_bin: Path) -> Path:
    """Append the MCP table. Leave a file that already names it unchanged."""
    path = dest / ".codex" / "config.toml"
    marker = "mcp_servers.trajectory-ir"
    try:
        current = path.read_text(encoding="utf-8") if path.is_file() else ""
    except OSError as exc:
        print(f"trajir attach: left Codex config unread ({exc})", file=sys.stderr)
        return path
    if marker in current:
        return path
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(current + _codex_block(mcp_bin, dest), encoding="utf-8")
    return path


def _snippet_header(sentence: str) -> str:
    return f'"""{sentence}\n\nImporting this file does not import the host library or DBOS.\n"""\n'


def _path_bootstrap(install: Path) -> str:
    root = repr(str(install))
    return (
        "    import sys\n"
        "    from pathlib import Path\n\n"
        f"    root = Path({root})\n"
        '    for entry in (root, root / "pkg"):\n'
        "        text = str(entry)\n"
        "        if text not in sys.path:\n"
        "            sys.path.insert(0, text)\n"
    )


def _langgraph_snippet(install: Path, dest: Path) -> str:
    work = repr(str(dest / ".local-evidence" / "langgraph-agent"))
    return (
        _snippet_header("LangGraph attach. Call attach() from the app you already run.")
        + '\ndef attach(tools, *, tenant_id="local", trajectory_id="langgraph-one-step", '
        + f"work_dir={work}):\n"
        + _path_bootstrap(install)
        + "    from integrations.langgraph import TrajIRToolGuard\n"
        + "    from langgraph.prebuilt import ToolNode\n\n"
        + "    guard = TrajIRToolGuard(\n"
        + "        tenant_id=tenant_id,\n"
        + "        trajectory_id=trajectory_id,\n"
        + "        work_dir=work_dir,\n"
        + "    )\n"
        + "    return ToolNode(tools, wrap_tool_call=guard.wrap_tool_call)\n"
    )


def _adk_snippet(install: Path, dest: Path) -> str:
    work = repr(str(dest / ".local-evidence" / "adk-agent"))
    return (
        _snippet_header("ADK attach. The callbacks seal and record. They do not call the tool.")
        + '\ndef callbacks(*, tenant_id="local", trajectory_id="adk-one-step", '
        + f"work_dir={work}):\n"
        + _path_bootstrap(install)
        + "    from integrations.adk import AdkToolGuard\n\n"
        + "    guard = AdkToolGuard(\n"
        + "        tenant_id=tenant_id,\n"
        + "        trajectory_id=trajectory_id,\n"
        + "        work_dir=work_dir,\n"
        + "    )\n"
        + "    return guard.before_tool_callback, guard.after_tool_callback\n"
    )


_GENERATED_IGNORES = (
    ".local-evidence/",
    ".claude/settings.local.json",
    ".mcp.json",
    ".cursor/mcp.json",
    ".vscode/mcp.json",
    ".gemini/settings.json",
    ".codex/config.toml",
    ".trajir/*.py",
)


def _ignore_line_present(current: str, needle: str) -> bool:
    lines = {line.strip() for line in current.splitlines()}
    if needle in lines:
        return True
    return needle == ".local-evidence/" and ".local-evidence" in lines


def _ignore_generated(dest: Path) -> None:
    """Keep generated absolute paths out of git. Add only the missing lines."""
    dest.mkdir(parents=True, exist_ok=True)
    path = dest / ".gitignore"
    try:
        current = path.read_text(encoding="utf-8") if path.is_file() else ""
    except OSError as exc:
        print(f"trajir attach: left .gitignore unread ({exc})", file=sys.stderr)
        return
    missing = [line for line in _GENERATED_IGNORES if not _ignore_line_present(current, line)]
    if not missing:
        return
    prefix = ""
    if current and not current.endswith("\n"):
        prefix = "\n"
    path.write_text(current + prefix + "\n".join(missing) + "\n", encoding="utf-8")


def write_local_config(
    root: Path,
    py: str,
    mcp_bin: Path,
    target: Path | None = None,
) -> int:
    """Merge TrajIR hooks and MCP config into one project directory.

    ``root`` is the Trajectory IR install. ``target`` is the project that
    receives the JSON. When target is omitted, both are the same directory.
    Both paths are resolved so a relative target is not stored as text.
    """
    install = Path(root).resolve()
    dest = Path(target).resolve() if target is not None else install
    mcp_bin = Path(mcp_bin).resolve()
    paths = {name: dest / rel for name, rel in _JSON_RELATIVE.items()}
    try:
        loaded = {name: _read_object(path) for name, path in paths.items()}
    except ValueError as exc:
        print(f"trajir claude config: {exc}", file=sys.stderr)
        return 1

    _ignore_generated(dest)
    work = dest / ".local-evidence" / "claude-agent"
    work.mkdir(parents=True, exist_ok=True)
    identity = {
        "work_dir": str(work),
        "tenant_id": "local",
        "trajectory_id": "claude-one-step",
    }
    ident_path = dest / ".local-evidence" / "claude-one-step.json"
    _write_json(ident_path, identity)
    _write_json(
        paths["settings"],
        _merged_settings(loaded["settings"], install, dest, py, ident_path),
    )
    entry = _server(mcp_bin, dest)
    _write_json(paths["mcp"], _merged_named(loaded["mcp"], "mcpServers", entry))
    _write_json(paths["cursor"], _merged_named(loaded["cursor"], "mcpServers", entry))
    vscode_entry = {"type": "stdio", **entry}
    _write_json(paths["vscode"], _merged_named(loaded["vscode"], "servers", vscode_entry))
    _write_json(paths["gemini"], _merged_named(loaded["gemini"], "mcpServers", entry))
    codex_path = _write_codex(dest, mcp_bin)
    snippet_dir = dest / ".trajir"
    snippet_dir.mkdir(parents=True, exist_ok=True)
    langgraph_path = snippet_dir / "langgraph_attach.py"
    adk_path = snippet_dir / "adk_attach.py"
    langgraph_path.write_text(_langgraph_snippet(install, dest), encoding="utf-8")
    adk_path.write_text(_adk_snippet(install, dest), encoding="utf-8")
    for path in (
        paths["settings"],
        paths["mcp"],
        paths["cursor"],
        paths["vscode"],
        paths["gemini"],
        ident_path,
        codex_path,
        langgraph_path,
        adk_path,
    ):
        print(path)
    return 0
