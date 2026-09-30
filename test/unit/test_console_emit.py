"""Console emit envelope and non-blocking failure."""

import json
from pathlib import Path

from client.python.trajectory_client import open_trajectory, project, seal_decision
from trajectory_ir.console_emit import FileSink, emit, note_package, note_seal
from trajectory_ir.package import export_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import Node

KINDS = (
    "node.appended",
    "seal.created",
    "export.completed",
    "import.completed",
)


class _Capture:
    def __init__(self):
        self.events = []

    def emit(self, event):
        self.events.append(event)


class _Boom:
    def emit(self, event):
        raise RuntimeError("disk full")


def test_kinds_and_required_fields():
    sink = _Capture()
    node = Node(
        kind="DECISION",
        trajectory_id="t-emit",
        tenant_id="demo",
        step_n=1,
        seq=1,
        payload={"plan": {"tool_calls": [{"name": "echo", "args": {}}]}},
    )
    note_seal(
        sink,
        trajectory_id="t-emit",
        tenant_id="demo",
        node=node,
        step_n=1,
        plan={"tool_calls": [{"name": "echo", "args": {}}]},
    )
    seal = sink.events[0]
    assert seal["kind"] == "seal.created"
    assert seal["kind"] in KINDS
    assert seal["schema_version"] == "console-events-v1"
    for key in ("node_id", "step_n", "content_hash"):
        assert key in seal["payload"]
    assert seal["payload"]["tool_names"] == ["echo"]

    note_package(
        sink,
        kind="export.completed",
        trajectory_id="t-emit",
        tenant_id="demo",
        path="out.tir",
        mode="thin",
        redacted=True,
        nbytes=12,
        member_count=5,
        node_count=1,
        ok=True,
    )
    exported = sink.events[1]
    assert exported["kind"] == "export.completed"
    for key in ("path", "mode", "redacted", "bytes", "member_count", "node_count", "ok"):
        assert key in exported["payload"]

    note_package(
        sink,
        kind="import.completed",
        trajectory_id="t-emit",
        tenant_id="demo",
        path="out.tir",
        mode="thin",
        redacted=True,
        nbytes=12,
        member_count=5,
        node_count=1,
        ok=False,
        error="HashMismatch",
    )
    imported = sink.events[2]
    assert imported["kind"] == "import.completed"
    assert imported["payload"]["verify_ok"] is False
    assert imported["payload"]["error"] == "HashMismatch"
    assert "ok" not in imported["payload"]


def test_file_sink_stages_tir(tmp_path: Path):
    src = tmp_path / "out.tir"
    src.write_bytes(b"tir-bytes")
    sink = FileSink(str(tmp_path))
    emit(
        sink,
        {
            "kind": "export.completed",
            "trajectory_id": "pack",
            "payload": {
                "path": str(src),
                "mode": "thin",
                "redacted": True,
                "bytes": 9,
                "member_count": 5,
                "node_count": 1,
                "ok": True,
            },
        },
    )
    copied = tmp_path / "packages" / "pack" / "out.tir"
    assert copied.read_bytes() == b"tir-bytes"
    raw = (tmp_path / "trajectories" / "pack.ndjson").read_text(encoding="utf-8")
    event = json.loads(raw)
    assert event["payload"]["console_path"] == "packages/pack/out.tir"


def test_file_sink_line(tmp_path: Path):
    sink = FileSink(str(tmp_path))
    emit(
        sink,
        {
            "kind": "node.appended",
            "trajectory_id": "file-t",
            "payload": {"node_id": "n", "kind": "COMMIT_STEP"},
        },
    )
    raw = (tmp_path / "trajectories" / "file-t.ndjson").read_text(encoding="utf-8")
    event = json.loads(raw)
    assert event["kind"] == "node.appended"
    assert event["schema_version"] == "console-events-v1"
    assert event["payload"]["node_id"] == "n"


def test_project_emits_context_projected(tmp_path: Path):
    sink = _Capture()
    traj = open_trajectory(
        "demo",
        "t-proj",
        db_path=str(tmp_path / "n.sqlite"),
        console_sink=sink,
    )
    try:
        project(traj, 1, {"goal": "x"})
    finally:
        traj.close()
    kinds = [event["kind"] for event in sink.events]
    assert "node.appended" in kinds
    assert "context.projected" in kinds


def test_sink_failure_does_not_fail_seal(tmp_path: Path):
    traj = open_trajectory(
        "demo",
        "t-sink",
        db_path=str(tmp_path / "n.sqlite"),
        console_sink=_Boom(),
    )
    try:
        seal_decision(traj, 1, {"tool_calls": [{"name": "echo", "args": {}}]})
    finally:
        traj.close()


def test_export_observer_failure(tmp_path: Path):
    log = NodeLog(str(tmp_path / "n.sqlite"))
    try:
        log.append("PROJECT_CONTEXT", 1, {"goal": "x"}, "t1", "demo", 0)

        def boom(_info):
            raise RuntimeError("nope")

        path = export_tir(log, "t1", tmp_path / "out.tir", mode="thin", on_exported=boom)
        assert Path(path).is_file()
    finally:
        log.close()
