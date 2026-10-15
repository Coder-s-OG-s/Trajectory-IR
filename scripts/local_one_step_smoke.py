"""Smoke the one-step seal, record, export, and verify path. Local only."""

from __future__ import annotations

import shutil
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[1]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.hosthook.session import record_tool_result, seal_before_tool  # noqa: E402
from integrations.langgraph.verify_pack import ensure_trajir_built, run_verify  # noqa: E402
from trajectory_ir.package import export_tir  # noqa: E402
from trajectory_ir.runtime.log import NodeLog  # noqa: E402


def main() -> int:
    work = _ROOT / ".local-evidence" / "one-step-smoke"
    if work.exists():
        shutil.rmtree(work)
    work.mkdir(parents=True)
    seal_before_tool(
        work,
        "local",
        "one-step-smoke",
        "echo",
        {"msg": "one-step"},
        "smoke-1",
        annotations={"readOnlyHint": True, "destructiveHint": False},
    )
    record_tool_result(work, "local", "one-step-smoke", "smoke-1", {"msg": "one-step"})
    dest = work / "smoke.tir"
    log = NodeLog(str(work / "nodes.sqlite"))
    try:
        export_tir(log, "one-step-smoke", dest, mode="thin", tenant_id="local")
    finally:
        log.close()
    exe = ensure_trajir_built(_ROOT)
    result = run_verify(dest, root=_ROOT)
    print(f"verify_ok={result.get('ok')} bin={exe}")
    if not result.get("ok"):
        print(result)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
