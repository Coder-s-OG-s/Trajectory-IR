"""Claude Code PreToolUse hook. Seals the decision, then exits.

Exit 0 lets the host run the tool. Exit 2 blocks the tool because the seal
did not land. The tool body is not called from this process.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[2]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.claude.config import hook_identity, tool_use_id  # noqa: E402
from integrations.hosthook.session import seal_before_tool  # noqa: E402


def main() -> int:
    raw = sys.stdin.read()
    try:
        event = json.loads(raw) if raw.strip() else {}
    except json.JSONDecodeError as exc:
        print(f"trajir pre-tool: bad json: {exc}", file=sys.stderr)
        return 2
    if not isinstance(event, dict):
        print("trajir pre-tool: event must be an object", file=sys.stderr)
        return 2
    name = str(event.get("tool_name") or "")
    tool_input = event.get("tool_input")
    args = tool_input if isinstance(tool_input, dict) else {"value": tool_input}
    try:
        ident = hook_identity()
        seal_before_tool(
            ident["work_dir"],
            ident["tenant_id"],
            ident["trajectory_id"],
            name,
            args,
            tool_use_id(event),
        )
    except Exception as exc:
        print(f"trajir pre-tool seal failed: {exc}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
