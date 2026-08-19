"""put_artifact -> thin export -> rehydrate end to end."""

from __future__ import annotations

from trajectory_ir.package.tir import export_tir, import_tir, load_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.storage import (
    CASNotFoundError,
    FileSystemCAS,
    put_artifact,
    rehydrate_artifacts,
)


def test_put_export_thin_rehydrate_roundtrip(tmp_path):
    cas = FileSystemCAS(tmp_path / "cas")
    db = str(tmp_path / "nodes.sqlite")
    log = NodeLog(db)
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)

    data = b"portable artifact payload"
    ref = put_artifact(cas, data, logical_path="outputs/result.bin")
    assert ref.content_hash == cas.put(data)  # idempotent
    assert ref.uri == f"cas://{ref.content_hash}"
    assert ref.size == len(data)

    dest = tmp_path / "run.tir"
    export_tir(
        log,
        "t1",
        dest,
        mode="thin",
        artifacts=[ref],
        tenant_id="demo",
        cas=cas,
    )

    pkg = load_tir(dest)
    assert pkg.manifest["mode"] == "thin"
    assert pkg.artifact_bytes == {}
    assert len(pkg.artifacts_manifest) == 1
    assert pkg.artifacts_manifest[0]["content_hash"] == ref.content_hash

    got = rehydrate_artifacts(cas, pkg.artifacts_manifest)
    assert got[ref.content_hash] == data

    # import with rehydrate fills package bytes for host convenience
    db2 = str(tmp_path / "nodes2.sqlite")
    log2 = NodeLog(db2)
    pkg2 = import_tir(dest, log2, cas=cas, rehydrate=True)
    assert pkg2.artifact_bytes[ref.content_hash] == data
    assert log2.has("t1", "demo", 1, "DECISION")
    log.close()
    log2.close()


def test_export_thin_with_cas_fails_if_missing(tmp_path):
    cas = FileSystemCAS(tmp_path / "cas")
    db = str(tmp_path / "nodes.sqlite")
    log = NodeLog(db)
    log.append("DECISION", 1, {"plan": {}}, "t1", "demo", 1)
    # Ref claims a hash that was never put
    from trajectory_ir.package.tir import ArtifactRef
    from trajectory_ir.storage.cas import content_hash

    fake = content_hash(b"not-in-store")
    ref = ArtifactRef(logical_path="x.bin", content_hash=fake, uri=f"cas://{fake}", size=1)
    try:
        export_tir(
            log,
            "t1",
            tmp_path / "bad.tir",
            mode="thin",
            artifacts=[ref],
            tenant_id="demo",
            cas=cas,
        )
        raise AssertionError("expected CASNotFoundError")
    except CASNotFoundError:
        pass
    log.close()


def test_put_artifact_requires_path(tmp_path):
    cas = FileSystemCAS(tmp_path / "cas")
    try:
        put_artifact(cas, b"x", logical_path="")
        raise AssertionError("expected ValueError")
    except ValueError:
        pass
