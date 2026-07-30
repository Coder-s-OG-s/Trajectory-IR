"""R05-style tests for .tir export / import."""

from __future__ import annotations

import json
import zipfile
from pathlib import Path

import pytest

from trajectory_ir.package import (
    ArtifactRef,
    TirVerificationError,
    export_tir,
    import_tir,
    load_tir,
)
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import payload_hash


@pytest.fixture
def sample_log(tmp_path: Path) -> NodeLog:
    log = NodeLog(str(tmp_path / "src.sqlite"))
    log.append(
        "PROJECT_CONTEXT",
        1,
        {"goal": "demo"},
        trajectory_id="t-export",
        tenant_id="demo",
        seq=0,
    )
    log.append(
        "DECISION",
        1,
        {"plan": {"tool_calls": [{"name": "echo", "args": {"msg": "hi"}}]}},
        trajectory_id="t-export",
        tenant_id="demo",
        seq=1,
    )
    log.append(
        "TOOL_CALL",
        1,
        {"tool": "echo", "args": {"msg": "hi"}},
        trajectory_id="t-export",
        tenant_id="demo",
        seq=2,
    )
    log.append(
        "TOOL_RESULT",
        1,
        {"result": "hi"},
        trajectory_id="t-export",
        tenant_id="demo",
        seq=3,
    )
    log.append(
        "COMMIT_STEP",
        1,
        {},
        trajectory_id="t-export",
        tenant_id="demo",
        seq=4,
    )
    yield log
    log.close()


def test_export_import_round_trip_thin(sample_log: NodeLog, tmp_path: Path) -> None:
    out = tmp_path / "run.tir"
    export_tir(sample_log, "t-export", out, mode="thin")
    assert out.is_file()

    dest = NodeLog(str(tmp_path / "dest.sqlite"))
    try:
        pkg = import_tir(out, dest)
        assert pkg.manifest["mode"] == "thin"
        assert pkg.manifest["trajectory_id"] == "t-export"
        assert len(pkg.nodes) == 5
        assert dest.count(pkg.nodes[0]["id"]) == 1
        # Round-trip preserves hashes
        for n in pkg.nodes:
            assert payload_hash(n["payload"]) == payload_hash(
                sample_log.list_nodes("t-export")[
                    next(
                        i
                        for i, x in enumerate(sample_log.list_nodes("t-export"))
                        if x["id"] == n["id"]
                    )
                ]["payload"]
            )
        reloaded = dest.list_nodes("t-export")
        assert len(reloaded) == 5
        assert [n["id"] for n in reloaded] == [n["id"] for n in sample_log.list_nodes("t-export")]
    finally:
        dest.close()


def test_export_import_round_trip_fat(sample_log: NodeLog, tmp_path: Path) -> None:
    blob = b"print('hello')\n"
    h = __import__("hashlib").sha256(blob).hexdigest()
    out = tmp_path / "fat.tir"
    export_tir(
        sample_log,
        "t-export",
        out,
        mode="fat",
        artifacts=[ArtifactRef(logical_path="src/main.py", content_hash=h)],
        artifact_bytes={h: blob},
    )
    pkg = load_tir(out)
    assert pkg.manifest["mode"] == "fat"
    assert h in pkg.artifact_bytes
    assert pkg.artifact_bytes[h] == blob


def test_import_detects_tampered_node(sample_log: NodeLog, tmp_path: Path) -> None:
    out = tmp_path / "good.tir"
    export_tir(sample_log, "t-export", out, mode="thin")

    bad = tmp_path / "bad.tir"
    with zipfile.ZipFile(out, "r") as zin, zipfile.ZipFile(bad, "w") as zout:
        for item in zin.infolist():
            data = zin.read(item.filename)
            if item.filename == "nodes.ndjson":
                lines = data.decode().splitlines()
                rec = json.loads(lines[1])
                rec["payload"] = {"plan": {"tool_calls": [{"name": "evil"}]}}
                # leave id as original so verification must fail
                lines[1] = json.dumps(rec, sort_keys=True)
                data = ("\n".join(lines) + "\n").encode()
            zout.writestr(item, data)

    with pytest.raises(TirVerificationError):
        load_tir(bad)


def test_idempotent_reimport(sample_log: NodeLog, tmp_path: Path) -> None:
    out = tmp_path / "run.tir"
    export_tir(sample_log, "t-export", out, mode="thin")
    dest = NodeLog(str(tmp_path / "dest.sqlite"))
    try:
        import_tir(out, dest)
        import_tir(out, dest)
        assert len(dest.list_nodes("t-export")) == 5
    finally:
        dest.close()


def test_thin_rejects_embedded_bytes(sample_log: NodeLog, tmp_path: Path) -> None:
    out = tmp_path / "weird.tir"
    export_tir(sample_log, "t-export", out, mode="thin")
    # Manually inject an artifact path into a thin package
    bad = tmp_path / "thin_with_bytes.tir"
    with zipfile.ZipFile(out, "r") as zin, zipfile.ZipFile(bad, "w") as zout:
        for item in zin.infolist():
            zout.writestr(item, zin.read(item.filename))
        h = "ab" + "0" * 62
        zout.writestr(f"artifacts/cas/ab/{h}", b"nope")
    # content hash of b"nope" won't match filename either — either error is fine
    with pytest.raises(TirVerificationError):
        load_tir(bad)
