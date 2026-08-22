"""R05-style tests for .tir export / import."""

from __future__ import annotations

import json
import zipfile
from pathlib import Path
from typing import Generator

import pytest

from trajectory_ir.package import (
    ArtifactRef,
    TirError,
    TirVerificationError,
    export_tir,
    import_tir,
    load_tir,
)
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import payload_hash


@pytest.fixture
def sample_log(tmp_path: Path) -> Generator[NodeLog, None, None]:
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
                sample_log.list_nodes("t-export", tenant_id="demo")[
                    next(
                        i
                        for i, x in enumerate(sample_log.list_nodes("t-export", tenant_id="demo"))
                        if x["id"] == n["id"]
                    )
                ]["payload"]
            )
        reloaded = dest.list_nodes("t-export", tenant_id="demo")
        assert len(reloaded) == 5
        assert [n["id"] for n in reloaded] == [
            n["id"] for n in sample_log.list_nodes("t-export", tenant_id="demo")
        ]
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
        assert len(dest.list_nodes("t-export", tenant_id="demo")) == 5
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


def test_load_tir_rejects_verify_false(sample_log: NodeLog, tmp_path: Path) -> None:
    out = tmp_path / "run.tir"
    export_tir(sample_log, "t-export", out, mode="thin")
    with pytest.raises(TirError, match="load_tir_unverified"):
        load_tir(out, verify=False)


def test_import_tir_rejects_verify_false(sample_log: NodeLog, tmp_path: Path) -> None:
    out = tmp_path / "run.tir"
    export_tir(sample_log, "t-export", out, mode="thin")
    dest = NodeLog(str(tmp_path / "dest.sqlite"))
    try:
        with pytest.raises(TirError, match="import_tir refuses"):
            import_tir(out, dest, verify=False)
    finally:
        dest.close()


def test_rejects_path_traversal_member(sample_log: NodeLog, tmp_path: Path) -> None:
    out = tmp_path / "good.tir"
    export_tir(sample_log, "t-export", out, mode="thin")
    bad = tmp_path / "traversal.tir"
    with zipfile.ZipFile(out, "r") as zin, zipfile.ZipFile(bad, "w") as zout:
        for item in zin.infolist():
            zout.writestr(item, zin.read(item.filename))
        zout.writestr("../evil.txt", b"x")
    with pytest.raises(TirVerificationError, match="unsafe"):
        load_tir(bad)


def test_redacted_export_strips_secrets(sample_log: NodeLog, tmp_path: Path) -> None:
    sample_log.append(
        "TOOL_CALL",
        2,
        {"tool": "login", "args": {"api_key": "super-secret", "user": "a"}},
        trajectory_id="t-export",
        tenant_id="demo",
        seq=10,
    )
    out = tmp_path / "redacted.tir"
    export_tir(sample_log, "t-export", out, mode="thin", redacted=True)
    pkg = load_tir(out)
    assert pkg.manifest.get("redacted") is True
    tool_calls = [n for n in pkg.nodes if n["kind"] == "TOOL_CALL" and n["seq"] == 10]
    assert len(tool_calls) == 1
    assert tool_calls[0]["payload"]["args"]["api_key"] == "[REDACTED]"
    assert tool_calls[0]["payload"]["args"]["user"] == "a"


def test_redacted_export_strips_missed_key_patterns(sample_log: NodeLog, tmp_path: Path) -> None:
    sample_log.append(
        "TOOL_CALL",
        2,
        {
            "tool": "login",
            "args": {
                "pass_phrase": "correct horse battery staple",
                "client_aws_access_key": "irrelevant-by-key",
                "user": "a",
            },
        },
        trajectory_id="t-export",
        tenant_id="demo",
        seq=11,
    )
    out = tmp_path / "redacted-keys.tir"
    export_tir(sample_log, "t-export", out, mode="thin", redacted=True)
    pkg = load_tir(out)
    tool_calls = [n for n in pkg.nodes if n["kind"] == "TOOL_CALL" and n["seq"] == 11]
    assert tool_calls[0]["payload"]["args"]["pass_phrase"] == "[REDACTED]"
    assert tool_calls[0]["payload"]["args"]["client_aws_access_key"] == "[REDACTED]"
    assert tool_calls[0]["payload"]["args"]["user"] == "a"


def test_redacted_export_strips_secret_shaped_values(sample_log: NodeLog, tmp_path: Path) -> None:
    sample_log.append(
        "TOOL_CALL",
        2,
        {
            "tool": "note",
            "args": {
                "note": "found it: AKIAABCDEFGHIJKLMNOP",
                "header": "Authorization: Bearer abcdefghijklmnopqrstuvwxyz",
                "message": "just a normal string",
            },
        },
        trajectory_id="t-export",
        tenant_id="demo",
        seq=12,
    )
    out = tmp_path / "redacted-values.tir"
    export_tir(sample_log, "t-export", out, mode="thin", redacted=True)
    pkg = load_tir(out)
    tool_calls = [n for n in pkg.nodes if n["kind"] == "TOOL_CALL" and n["seq"] == 12]
    args = tool_calls[0]["payload"]["args"]
    # Neither "note" nor "header" match _SECRET_KEY_RE; both are caught by
    # the value-shape pass (AWS access key id, bearer token).
    assert args["note"] == "[REDACTED]"
    assert args["header"] == "[REDACTED]"
    assert args["message"] == "just a normal string"


def test_export_tenant_scope(sample_log: NodeLog, tmp_path: Path) -> None:
    other = NodeLog(str(tmp_path / "mixed.sqlite"))
    try:
        other.append("DECISION", 1, {"plan": "a"}, "shared", "tenant-a", 1)
        other.append("DECISION", 1, {"plan": "b"}, "shared", "tenant-b", 1)
        out = tmp_path / "scoped.tir"
        export_tir(other, "shared", out, mode="thin", tenant_id="tenant-a")
        pkg = load_tir(out)
        assert len(pkg.nodes) == 1
        assert pkg.nodes[0]["tenant_id"] == "tenant-a"
    finally:
        other.close()
