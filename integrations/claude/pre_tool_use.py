"""Claude Code PreToolUse hook. Seals the decision, then exits.

Exit 0 lets the host run the tool the first time. The same hook owner
asking again while that call is pending exits 2 when the effect is gated
or missing. That is a crash before PostToolUse. A different owner on the
pending call exits 0, so two registrations do not block the first run.
Exit 2 also blocks when the seal did not land, or when this tool_use_id
was already recorded and the effect is gated or unknown. The tool body
is not called from this process.
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[2]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.claude.config import hook_identity, tool_use_id  # noqa: E402
from integrations.hosthook.session import seal_before_tool  # noqa: E402

from trajectory_ir.effects.classify import EffectClass, requires_block_and_gate  # noqa: E402


def _annotations(event: dict) -> dict | None:
    raw = event.get("annotations")
    if isinstance(raw, dict):
        return raw
    raw = event.get("tool_annotations")
    if isinstance(raw, dict):
        return raw
    return None


def _replay_blocked(record: dict) -> bool:
    """Same slot as LangGraph block-and-gate: only a sealed non-idempotent write."""
    if not record.get("replay"):
        return False
    try:
        effect = EffectClass(str(record.get("effect_class") or ""))
    except ValueError:
        return True
    return requires_block_and_gate(effect)


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
        record = seal_before_tool(
            ident["work_dir"],
            ident["tenant_id"],
            ident["trajectory_id"],
            name,
            args,
            tool_use_id(event),
            annotations=_annotations(event),
            owner=os.environ.get("TRAJIR_HOOK_OWNER", "").strip(),
        )
    except Exception as exc:
        print(f"trajir pre-tool seal failed: {exc}", file=sys.stderr)
        return 2
    if _replay_blocked(record):
        print(
            "trajir pre-tool: already sealed, not running this tool again",
            file=sys.stderr,
        )
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
