"""Conformance test for R05: .tir export/import preserves node hashes and seals.

README §10 R05 — Export and re-import of a `.tir` package preserves all node
hashes and seals exactly. This is the named gate for the portable package
format after thin/fat export landed in Python and Go.
"""

from __future__ import annotations

import hashlib
from pathlib import Path

from trajectory_ir.package import ArtifactRef, export_tir, import_tir, load_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import payload_hash


def _seed_trajectory(log: NodeLog, trajectory_id: str = "r05-traj") -> None:
    log.append(
        "PROJECT_CONTEXT",
        1,
        {"goal": "r05-conformance"},
        trajectory_id=trajectory_id,
        tenant_id="demo",
        seq=0,
    )
    log.append(
        "DECISION",
        1,
        {"plan": {"tool_calls": [{"name": "echo", "args": {"msg": "r05"}}]}},
        trajectory_id=trajectory_id,
        tenant_id="demo",
        seq=1,
    )
    log.append(
        "TOOL_CALL",
        1,
        {"tool": "echo", "args": {"msg": "r05"}},
        trajectory_id=trajectory_id,
        tenant_id="demo",
        seq=2,
    )
    log.append(
        "TOOL_RESULT",
        1,
        {"result": "r05"},
        trajectory_id=trajectory_id,
        tenant_id="demo",
        seq=3,
    )
    log.append(
        "COMMIT_STEP",
        1,
        {},
        trajectory_id=trajectory_id,
        tenant_id="demo",
        seq=4,
    )


def test_r05_thin_export_import_preserves_node_ids_and_seals(tmp_path: Path) -> None:
    """R05 thin: every node id and DECISION seal content_hash survives round-trip."""
    src = NodeLog(str(tmp_path / "src.sqlite"))
    try:
        _seed_trajectory(src)
        original = src.list_nodes("r05-traj")
        original_ids = [n["id"] for n in original]
        original_decision_hashes = {
            n["id"]: payload_hash(n["payload"]) for n in original if n["kind"] == "DECISION"
        }

        pkg_path = tmp_path / "r05-thin.tir"
        export_tir(src, "r05-traj", pkg_path, mode="thin")

        dest = NodeLog(str(tmp_path / "dest.sqlite"))
        try:
            pkg = import_tir(pkg_path, dest)
            reloaded = dest.list_nodes("r05-traj")
            assert [n["id"] for n in reloaded] == original_ids
            assert len(pkg.seals) == len(original_decision_hashes)
            for seal in pkg.seals:
                assert seal["node_id"] in original_decision_hashes
                assert seal["content_hash"] == original_decision_hashes[seal["node_id"]]
            # Seals in package must match recomputation from imported nodes
            for n in reloaded:
                if n["kind"] == "DECISION":
                    assert payload_hash(n["payload"]) == original_decision_hashes[n["id"]]
        finally:
            dest.close()
    finally:
        src.close()


def test_r05_fat_export_import_preserves_artifact_hashes(tmp_path: Path) -> None:
    """R05 fat: embedded artifact content hashes and node ids round-trip."""
    src = NodeLog(str(tmp_path / "src.sqlite"))
    try:
        _seed_trajectory(src, trajectory_id="r05-fat")
        blob = b"r05-artifact-bytes\n"
        h = hashlib.sha256(blob).hexdigest()
        pkg_path = tmp_path / "r05-fat.tir"
        export_tir(
            src,
            "r05-fat",
            pkg_path,
            mode="fat",
            artifacts=[ArtifactRef(logical_path="blob.txt", content_hash=h)],
            artifact_bytes={h: blob},
        )

        loaded = load_tir(pkg_path)
        assert loaded.manifest["mode"] == "fat"
        assert loaded.artifact_bytes[h] == blob
        assert hashlib.sha256(loaded.artifact_bytes[h]).hexdigest() == h

        dest = NodeLog(str(tmp_path / "dest.sqlite"))
        try:
            pkg = import_tir(pkg_path, dest)
            original_ids = [n["id"] for n in src.list_nodes("r05-fat")]
            assert [n["id"] for n in dest.list_nodes("r05-fat")] == original_ids
            assert pkg.manifest["trajectory_id"] == "r05-fat"
        finally:
            dest.close()
    finally:
        src.close()


def test_r05_go_golden_fixture_loads_with_matching_ids(tmp_path: Path) -> None:
    """R05 cross-language: Python load of the committed Go/Python golden fixture.

    `testdata/sample_thin.tir` is produced by the Python exporter and imported
    by Go tests; this asserts the same fixture still verifies under Python.
    """
    root = Path(__file__).resolve().parents[1]
    golden = root / "testdata" / "sample_thin.tir"
    assert golden.is_file(), f"missing golden fixture {golden}"

    pkg = load_tir(golden)
    assert pkg.manifest.get("mode") == "thin"
    assert len(pkg.nodes) >= 1
    # load_tir already re-verified every node id; re-check DECISION seals.
    decision_ids = {n["id"] for n in pkg.nodes if n["kind"] == "DECISION"}
    seal_ids = {s["node_id"] for s in pkg.seals}
    assert decision_ids <= seal_ids or decision_ids == seal_ids

    dest = NodeLog(str(tmp_path / "from-golden.sqlite"))
    try:
        import_tir(golden, dest)
        traj = pkg.manifest["trajectory_id"]
        assert [n["id"] for n in dest.list_nodes(traj)] == [n["id"] for n in pkg.nodes]
    finally:
        dest.close()
