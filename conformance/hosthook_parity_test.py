"""Go and Python host hook sessions write the same nodes.

The Go port (go/trajir/hosthook) is what agent plugins run. The Python
session (integrations/hosthook/session.py) is the reference. Both run the
event sequences in go/trajir/hosthook/testdata/sequences and must produce
the node ids in go/trajir/hosthook/testdata/parity, which the Go test
writes with -update.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from integrations.hosthook.session import (
    record_tool_failure,
    record_tool_result,
    seal_before_tool,
)

from trajectory_ir.runtime.log import NodeLog

_DATA = Path(__file__).resolve().parents[1] / "go" / "trajir" / "hosthook" / "testdata"
_SEQUENCES = sorted((_DATA / "sequences").glob("*.json"))


@pytest.mark.parametrize("path", _SEQUENCES, ids=lambda p: p.stem)
def test_python_session_matches_go_golden(path: Path, tmp_path: Path) -> None:
    steps = json.loads(path.read_text(encoding="utf-8"))
    for step in steps:
        op, tool_use_id = step["op"], step["id"]
        if op == "seal":
            seal_before_tool(
                tmp_path,
                "local",
                "parity",
                step["tool"],
                step.get("args") or {},
                tool_use_id,
                owner=step.get("owner", ""),
            )
        elif op == "result":
            record_tool_result(tmp_path, "local", "parity", tool_use_id, step.get("result"))
        elif op == "failure":
            record_tool_failure(tmp_path, "local", "parity", tool_use_id, step.get("error"))
        else:
            raise AssertionError(f"unknown op {op!r}")

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        got = [
            {"kind": n["kind"], "step_n": n["step_n"], "seq": n["seq"], "id": n["id"]}
            for n in log.list_nodes("parity", tenant_id="local")
        ]
    finally:
        log.close()
    golden = json.loads((_DATA / "parity" / f"{path.stem}.golden.json").read_text(encoding="utf-8"))
    assert got == golden


def test_sequences_present() -> None:
    assert len(_SEQUENCES) >= 4
