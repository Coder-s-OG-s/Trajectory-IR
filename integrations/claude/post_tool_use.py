"""Claude Code PostToolUse hook. Records the result the host already produced.

The tool has already run. A failure here is written to stderr and still exits
0 so the host is not asked to run the tool again.
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
from integrations.hosthook.session import record_tool_result  # noqa: E402


def main() -> int:
    raw = sys.stdin.read()
    try:
        event = json.loads(raw) if raw.strip() else {}
    except json.JSONDecodeError as exc:
        print(f"trajir post-tool: bad json: {exc}", file=sys.stderr)
        return 0
    if not isinstance(event, dict):
        print("trajir post-tool: event must be an object", file=sys.stderr)
        return 0
    result = event.get("tool_response", event.get("tool_output"))
    try:
        ident = hook_identity()
        record_tool_result(
            ident["work_dir"],
            ident["tenant_id"],
            ident["trajectory_id"],
            tool_use_id(event),
            result,
        )
    except Exception as exc:
        print(f"trajir post-tool record failed: {exc}", file=sys.stderr)
        return 0
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
