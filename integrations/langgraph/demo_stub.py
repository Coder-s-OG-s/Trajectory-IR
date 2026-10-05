"""Stub demo: seal → tool → .tir → ready for trajir verify. No paid API.

Run from repo root (worktree)::

    python integrations/langgraph/demo_stub.py
    go/trajir.exe verify .local-evidence/langgraph-demo.tir
"""

from __future__ import annotations

import argparse
import shutil
import sys
from pathlib import Path

# Repo root on sys.path when run as a script.
_ROOT = Path(__file__).resolve().parents[2]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.langgraph.sidecar import TrajIRToolGuard  # noqa: E402
from trajectory_ir.effects import EffectClass  # noqa: E402


def echo(msg: str) -> str:
    return msg


def ship_release(service: str, version: str) -> dict:
    return {"shipped": f"{service}:{version}"}


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

    with TrajIRToolGuard(
        tenant_id="demo",
        trajectory_id="langgraph-demo",
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
    print("next: go/trajir.exe verify", path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
