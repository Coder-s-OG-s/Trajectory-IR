"""Claude Code PostToolUseFailure hook. Records a tool the host already failed.

The tool has already failed. A failure here is written to stderr and still
exits 0 so the host is not asked to run the tool again.
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
from integrations.hosthook.session import record_tool_failure  # noqa: E402


def _failure_detail(event: dict) -> object:
    for key in ("error", "error_message", "tool_response"):
        value = event.get(key)
        if value not in (None, ""):
            return value
    return "host reported a tool failure"


def main() -> int:
    raw = sys.stdin.read()
    try:
        event = json.loads(raw) if raw.strip() else {}
    except json.JSONDecodeError as exc:
        print(f"trajir post-tool-failure: bad json: {exc}", file=sys.stderr)
        return 0
    if not isinstance(event, dict):
        print("trajir post-tool-failure: event must be an object", file=sys.stderr)
        return 0
    try:
        ident = hook_identity()
        record_tool_failure(
            ident["work_dir"],
            ident["tenant_id"],
            ident["trajectory_id"],
            tool_use_id(event),
            _failure_detail(event),
        )
    except Exception as exc:
        print(f"trajir post-tool-failure record failed: {exc}", file=sys.stderr)
        return 0
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
