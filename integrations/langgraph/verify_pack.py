"""Run ``trajir verify`` and optionally emit ``audit.completed`` to the console sink.

Shared by ``demo_stub`` and integration tests so verify + audit emit do not drift.
"""

from __future__ import annotations

import json
import subprocess
from pathlib import Path
from typing import Any

from trajectory_ir.console_emit import from_env, note_audit

_ROOT = Path(__file__).resolve().parents[2]


def trajir_bin(root: Path | None = None) -> Path:
    base = root or _ROOT
    exe = base / "go" / "trajir.exe"
    if exe.is_file():
        return exe
    return base / "go" / "trajir"


def ensure_trajir_built(root: Path | None = None) -> Path:
    """Build go/trajir.exe when missing. Returns path to the binary."""
    base = root or _ROOT
    exe = base / "go" / "trajir.exe"
    if exe.is_file():
        return exe
    subprocess.check_call(
        ["go", "build", "-o", str(exe), "./cmd/trajir"],
        cwd=str(base / "go"),
    )
    return exe


def run_verify(pack: Path | str, *, root: Path | None = None, timeout: float = 60) -> dict[str, Any]:
    """Run ``trajir verify --json`` and return the Result object."""
    base = root or _ROOT
    pack_path = Path(pack)
    bin_path = trajir_bin(base)
    if bin_path.is_file():
        cmd = [str(bin_path), "verify", "--json", str(pack_path)]
        cwd = None
    else:
        cmd = ["go", "run", "./cmd/trajir", "verify", "--json", str(pack_path)]
        cwd = str(base / "go")
    try:
        proc = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            cwd=cwd,
            check=False,
            timeout=timeout,
        )
    except subprocess.TimeoutExpired as exc:
        return {
            "path": str(pack_path),
            "ok": False,
            "findings": [
                {
                    "code": "VERIFY_INVOKE_FAILED",
                    "message": f"trajir verify timed out after {exc.timeout}s",
                }
            ],
        }
    raw = (proc.stdout or "").strip()
    if not raw:
        return {
            "path": str(pack_path),
            "ok": False,
            "findings": [
                {
                    "code": "VERIFY_INVOKE_FAILED",
                    "message": (proc.stderr or "no output").strip()[:500],
                }
            ],
        }
    try:
        data = json.loads(raw)
    except json.JSONDecodeError:
        return {
            "path": str(pack_path),
            "ok": False,
            "findings": [{"code": "VERIFY_JSON_PARSE", "message": raw[:500]}],
        }
    if not isinstance(data, dict):
        return {
            "path": str(pack_path),
            "ok": False,
            "findings": [{"code": "VERIFY_JSON_PARSE", "message": "result was not an object"}],
        }
    return data


def verify_and_note_audit(
    pack: Path | str,
    *,
    trajectory_id: str,
    tenant_id: str,
    sink: Any | None = None,
    root: Path | None = None,
) -> dict[str, Any]:
    """Verify pack; emit ``audit.completed`` when a console sink is available."""
    result = run_verify(pack, root=root)
    active = sink if sink is not None else from_env()
    if active is not None:
        note_audit(
            active,
            trajectory_id=trajectory_id,
            tenant_id=tenant_id,
            path=str(result.get("path") or pack),
            ok=bool(result.get("ok")),
            findings=result.get("findings") or [],
        )
    return result
