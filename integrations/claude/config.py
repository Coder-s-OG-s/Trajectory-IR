"""Local identity for the Claude hooks. No secrets."""

from __future__ import annotations

import json
import os
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[2]
_CONFIG = _ROOT / ".local-evidence" / "claude-one-step.json"


def hook_identity() -> dict[str, str]:
    file_cfg: dict[str, str] = {}
    if _CONFIG.is_file():
        loaded = json.loads(_CONFIG.read_text(encoding="utf-8"))
        if isinstance(loaded, dict):
            file_cfg = {str(k): str(v) for k, v in loaded.items()}
    work = os.environ.get("TRAJIR_WORK_DIR") or file_cfg.get("work_dir")
    if not work:
        work = str(_ROOT / ".local-evidence" / "claude-agent")
    return {
        "work_dir": work,
        "tenant_id": os.environ.get("TRAJIR_TENANT_ID") or file_cfg.get("tenant_id") or "local",
        "trajectory_id": os.environ.get("TRAJIR_TRAJECTORY_ID")
        or file_cfg.get("trajectory_id")
        or "claude-one-step",
    }


def tool_use_id(event: dict) -> str:
    raw = event.get("tool_use_id")
    if raw:
        return str(raw)
    raise ValueError("tool_use_id is required")
