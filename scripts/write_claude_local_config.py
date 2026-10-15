"""Write the project-local Claude hook and MCP config. Does not touch the user profile."""

from __future__ import annotations

import json
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[1]


def main() -> int:
    py = sys.executable
    pre = _ROOT / "integrations" / "claude" / "pre_tool_use.py"
    post = _ROOT / "integrations" / "claude" / "post_tool_use.py"
    mcp = _ROOT / "go" / "trajir-mcp.exe"
    if not mcp.is_file():
        alt = _ROOT / "go" / "trajir-mcp"
        mcp = alt if alt.is_file() else mcp
    work = _ROOT / ".local-evidence" / "claude-agent"
    work.mkdir(parents=True, exist_ok=True)
    ident = {
        "work_dir": str(work),
        "tenant_id": "local",
        "trajectory_id": "claude-one-step",
    }
    ident_path = _ROOT / ".local-evidence" / "claude-one-step.json"
    ident_path.parent.mkdir(parents=True, exist_ok=True)
    ident_path.write_text(json.dumps(ident, indent=2), encoding="utf-8")

    def command(script: Path) -> str:
        return f"\"{py}\" \"{script}\""

    settings = {
        "hooks": {
            "PreToolUse": [
                {
                    "matcher": "*",
                    "hooks": [{"type": "command", "command": command(pre)}],
                }
            ],
            "PostToolUse": [
                {
                    "matcher": "*",
                    "hooks": [{"type": "command", "command": command(post)}],
                }
            ],
        }
    }
    claude_dir = _ROOT / ".claude"
    claude_dir.mkdir(parents=True, exist_ok=True)
    (claude_dir / "settings.local.json").write_text(json.dumps(settings, indent=2), encoding="utf-8")
    mcp_cfg = {
        "mcpServers": {
            "trajectory-ir": {
                "command": str(mcp),
                "args": [],
                "env": {"TRAJIR_MCP_ROOT": str(_ROOT)},
            }
        }
    }
    (_ROOT / ".mcp.json").write_text(json.dumps(mcp_cfg, indent=2), encoding="utf-8")
    print(claude_dir / "settings.local.json")
    print(_ROOT / ".mcp.json")
    print(ident_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
