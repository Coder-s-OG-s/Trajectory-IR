"""One local soak pass: seal, crash, resume, verify, flip one byte.

This is a single pass on this machine. It is not a week of host use.
Provider token columns stay none. The console column is ceil(chars/4).
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[1]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.langgraph.verify_pack import ensure_trajir_built, run_verify  # noqa: E402
from trajectory_ir.package import export_tir  # noqa: E402
from trajectory_ir.runtime.log import NodeLog  # noqa: E402
from trajectory_ir.runtime.nodes import payload_hash  # noqa: E402


WORK = _ROOT / ".local-evidence" / "soak"
PACK = WORK / "soak.tir"
BROKEN = WORK / "soak-flipped.tir"
REPORT = _ROOT / "docs" / "LOCAL_SOAK_REPORT.md"


def _child(phase: str) -> int:
    from integrations.hosthook.session import record_tool_result, seal_before_tool

    if phase == "seal":
        if WORK.exists():
            shutil.rmtree(WORK)
        WORK.mkdir(parents=True)
        sealed = seal_before_tool(
            WORK,
            "local",
            "soak",
            "echo",
            {"msg": "soak"},
            "soak-1",
            annotations={"readOnlyHint": True, "destructiveHint": False},
        )
        (WORK / "sealed.json").write_text(json.dumps(sealed), encoding="utf-8")
        os._exit(42)
    if phase == "resume":
        record_tool_result(WORK, "local", "soak", "soak-1", {"msg": "recorded-after-crash"})
        nxt = seal_before_tool(
            WORK,
            "local",
            "soak",
            "echo",
            {"msg": "second"},
            "soak-2",
            annotations={"readOnlyHint": True, "destructiveHint": False},
        )
        record_tool_result(WORK, "local", "soak", "soak-2", {"msg": "second-step"})
        (WORK / "resumed.json").write_text(json.dumps({"next_step": nxt["step_n"]}), encoding="utf-8")
        return 0
    raise SystemExit(f"unknown phase {phase}")


def _estimate(rows: list[dict]) -> int | None:
    for row in rows:
        if row.get("kind") == "PROJECT_CONTEXT":
            raw = json.dumps(row.get("payload") or {}, sort_keys=True, separators=(",", ":"))
            return (len(raw) + 3) // 4
    return None


def _write_report(rows: list[dict], good: dict, bad: dict) -> None:
    decision = next(row for row in rows if row["kind"] == "DECISION")
    call = next(row for row in rows if row["kind"] == "TOOL_CALL")
    payload = decision.get("payload") if isinstance(decision.get("payload"), dict) else {}
    lines = [
        "# Local soak report",
        "",
        "One pass on this machine. This is not a week of use on a live host.",
        "",
        "1. The first process sealed the decision and then exited 42 before the result was recorded.",
        "2. The second process opened the same work directory, recorded the result, and sealed the next step.",
        "3. trajir verify on the exported pack passed.",
        "4. One flipped byte failed verify.",
        "",
        "The resume did not call the model again and did not rerun a tool body. The recorded result is the value the resume process supplied. That is at-most-one automatic attempt for a gated tool, and it is not exactly-once.",
        "",
        "| Column | Value |",
        "| --- | --- |",
        "| Provider prompt_tokens | none |",
        "| Provider completion_tokens | none |",
        f"| Console estimate ceil(chars/4) on the first PROJECT_CONTEXT | {_estimate(rows)} |",
        f"| Decision step | {decision.get('step_n')} |",
        f"| Resumed next step | {json.loads((WORK / 'resumed.json').read_text(encoding='utf-8'))['next_step']} |",
        f"| Who (tenant_id) | {decision.get('tenant_id')} |",
        f"| When (node ts, not part of the hash) | {decision.get('ts')} |",
        f"| Decision node id | {decision.get('id')} |",
        f"| Decision content hash | {payload_hash(payload)} |",
        f"| Tool | {call['payload'].get('tool')} |",
        "| Effect class | READ_ONLY (readOnlyHint on this echo; Bash would be NON_IDEMPOTENT_WRITE) |",
        f"| Idempotency key | {call['payload'].get('idempotency_key')} |",
        f"| verify ok | {good.get('ok')} |",
        f"| flipped verify ok | {bad.get('ok')} |",
        "",
        "Provider tokens stay none because this pass did not call a model provider.",
        "",
    ]
    REPORT.parent.mkdir(parents=True, exist_ok=True)
    REPORT.write_text("\n".join(lines), encoding="utf-8")


def main() -> int:
    if len(sys.argv) == 3 and sys.argv[1] == "--phase":
        return _child(sys.argv[2])
    py = sys.executable
    seal = subprocess.run([py, str(Path(__file__)), "--phase", "seal"], cwd=str(_ROOT))
    if seal.returncode != 42:
        print(f"seal process exited {seal.returncode}, expected 42")
        return 1
    resume = subprocess.run([py, str(Path(__file__)), "--phase", "resume"], cwd=str(_ROOT))
    if resume.returncode != 0:
        print(f"resume process exited {resume.returncode}")
        return 1
    log = NodeLog(str(WORK / "nodes.sqlite"))
    try:
        rows = log.list_nodes("soak", tenant_id="local")
        export_tir(log, "soak", PACK, mode="thin", tenant_id="local")
    finally:
        log.close()
    ensure_trajir_built(_ROOT)
    good = run_verify(PACK, root=_ROOT)
    blob = bytearray(PACK.read_bytes())
    blob[min(80, len(blob) - 1)] ^= 0x01
    BROKEN.write_bytes(blob)
    bad = run_verify(BROKEN, root=_ROOT)
    _write_report(rows, good, bad)
    print(f"verify_ok={good.get('ok')} flipped_ok={bad.get('ok')} report={REPORT}")
    if not good.get("ok") or bad.get("ok"):
        print(good)
        print(bad)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
