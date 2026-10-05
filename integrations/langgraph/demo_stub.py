"""Stub demo: seal → tool → .tir → trajir verify → optional console audit.

Run from repo root (worktree)::

    python integrations/langgraph/demo_stub.py
    go/trajir.exe verify .local-evidence/langgraph-demo.tir

With console sink (Evidence tab)::

    $env:TRAJIR_CONSOLE_SINK = "http"   # or file
    $env:TRAJIR_CONSOLE_URL = "http://127.0.0.1:8787"
    $env:TRAJIR_CONSOLE_DATA = ".console-data"   # when sink=file
    python integrations/langgraph/demo_stub.py
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

# Repo root on sys.path when run as a script.
_ROOT = Path(__file__).resolve().parents[2]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.langgraph.sidecar import TrajIRToolGuard  # noqa: E402
from trajectory_ir.console_emit import from_env, note_audit  # noqa: E402
from trajectory_ir.effects import EffectClass  # noqa: E402


def echo(msg: str) -> str:
    return msg


def ship_release(service: str, version: str) -> dict:
    return {"shipped": f"{service}:{version}"}


def _trajir_bin() -> Path:
    exe = _ROOT / "go" / "trajir.exe"
    if exe.is_file():
        return exe
    return _ROOT / "go" / "trajir"


def _run_verify(pack: Path) -> dict:
    """Run trajir verify --json; return Result dict (ok/findings/path)."""
    bin_path = _trajir_bin()
    cmd = [str(bin_path), "verify", "--json", str(pack)]
    if not bin_path.is_file():
        # Fall back to go run when binary is missing.
        cmd = ["go", "run", "./cmd/trajir", "verify", "--json", str(pack)]
        cwd = str(_ROOT / "go")
    else:
        cwd = None
    try:
        proc = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            cwd=cwd,
            check=False,
            timeout=60,
        )
    except subprocess.TimeoutExpired as exc:
        return {
            "path": str(pack),
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
            "path": str(pack),
            "ok": False,
            "findings": [
                {
                    "code": "VERIFY_INVOKE_FAILED",
                    "message": (proc.stderr or "no output").strip()[:500],
                }
            ],
        }
    try:
        return json.loads(raw)
    except json.JSONDecodeError:
        return {
            "path": str(pack),
            "ok": False,
            "findings": [
                {
                    "code": "VERIFY_JSON_PARSE",
                    "message": raw[:500],
                }
            ],
        }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--work-dir",
        default=str(_ROOT / ".local-evidence" / "langgraph-run"),
        help="directory for nodes.sqlite",
    )
    parser.add_argument(
        "--tir-out",
        default=str(_ROOT / ".local-evidence" / "langgraph-demo.tir"),
        help="thin package path",
    )
    parser.add_argument(
        "--skip-verify",
        action="store_true",
        help="skip trajir verify + audit.completed emit",
    )
    args = parser.parse_args()

    work = Path(args.work_dir)
    # Fresh NodeLog each run so NON_IDEMPOTENT_WRITE does not hit BLOCKED_NEEDS_GATE
    # from a previous interrupted attempt at the same seq.
    if work.exists():
        shutil.rmtree(work)
    work.mkdir(parents=True, exist_ok=True)
    out = Path(args.tir_out)
    out.parent.mkdir(parents=True, exist_ok=True)

    plan = [
        {"name": "echo", "args": {"msg": "hello-evidence"}},
        {
            "name": "ship_release",
            "args": {"service": "api", "version": "1.0.0"},
        },
    ]

    tenant_id = "demo"
    trajectory_id = "langgraph-demo"

    with TrajIRToolGuard(
        tenant_id=tenant_id,
        trajectory_id=trajectory_id,
        work_dir=work,
        context={"goal": "evidence sidecar stub", "host": "langgraph-stub"},
        effect_hints={
            "echo": EffectClass.PURE,
            "ship_release": EffectClass.NON_IDEMPOTENT_WRITE,
        },
    ) as guard:
        guard.run_sealed("echo", plan[0]["args"], echo, plan_calls=plan)
        guard.run_sealed(
            "ship_release",
            plan[1]["args"],
            ship_release,
            plan_calls=plan,
        )
        path = guard.export_tir(out)

    print("sealed step + thin package")
    print(f"  tir: {path}")

    if args.skip_verify:
        print("next: go/trajir.exe verify", path)
        return 0

    result = _run_verify(Path(path))
    ok = bool(result.get("ok"))
    findings = result.get("findings") or []
    print("trajir verify:", "OK" if ok else "FAIL", path)
    for f in findings:
        if isinstance(f, dict):
            print(f"  {f.get('code')}: {f.get('message')}")
        else:
            print(f"  {f}")

    sink = from_env()
    if sink is not None:
        note_audit(
            sink,
            trajectory_id=trajectory_id,
            tenant_id=tenant_id,
            path=str(path),
            ok=ok,
            findings=findings,
        )
        sink_kind = os.environ.get("TRAJIR_CONSOLE_SINK", "").strip() or "set"
        print(f"console: audit.completed via {sink_kind} sink")
    else:
        print("console: TRAJIR_CONSOLE_SINK unset (no audit.completed emit)")

    return 0 if ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
