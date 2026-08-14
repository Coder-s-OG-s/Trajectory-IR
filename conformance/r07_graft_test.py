"""Conformance R07: graft transfers artifact refs only, never THOUGHT."""

from __future__ import annotations

import pytest

from trajectory_ir.runtime.graft import GraftError, graft_artifact_ref
from trajectory_ir.runtime.log import NodeLog


def test_r07_graft_artifact_ref_not_thoughts(tmp_path) -> None:
    src = NodeLog(str(tmp_path / "src.sqlite"))
    dst = NodeLog(str(tmp_path / "dst.sqlite"))
    try:
        h = "ab" + "0" * 62
        src.append(
            "THOUGHT",
            1,
            {"text": "private reasoning about secret deploy"},
            "src-traj",
            "demo",
            0,
        )
        src.append(
            "ARTIFACT_PUT",
            1,
            {"content_hash": h, "logical_path": "out.bin", "uri": "cas://" + h},
            "src-traj",
            "demo",
            1,
        )
        source_nodes = src.list_nodes("src-traj", tenant_id="demo")
        assert any(n["kind"] == "THOUGHT" for n in source_nodes)

        node = graft_artifact_ref(
            dst,
            content_hash=h,
            target_trajectory_id="dst-traj",
            target_tenant_id="demo",
            seq=0,
            step_n=1,
            source_nodes=source_nodes,
        )
        assert node.kind == "ARTIFACT_REF"
        assert node.payload["content_hash"] == h
        assert node.payload.get("grafted") is True
        assert node.payload.get("logical_path") == "out.bin"

        grafted = dst.list_nodes("dst-traj", tenant_id="demo")
        kinds = {n["kind"] for n in grafted}
        assert "ARTIFACT_REF" in kinds
        assert "THOUGHT" not in kinds
        for n in grafted:
            assert "private reasoning" not in str(n.get("payload"))
    finally:
        src.close()
        dst.close()


def test_r07_refuses_graft_from_thought_payload_hash(tmp_path) -> None:
    """If the only match would be a private kind, refuse."""
    src = NodeLog(str(tmp_path / "src.sqlite"))
    dst = NodeLog(str(tmp_path / "dst.sqlite"))
    try:
        h = "cd" + "1" * 62
        src.append(
            "THOUGHT",
            1,
            {"content_hash": h, "text": "should not graft"},
            "src-traj",
            "demo",
            0,
        )
        with pytest.raises(GraftError, match=r"refusing|no ARTIFACT"):
            graft_artifact_ref(
                dst,
                content_hash=h,
                target_trajectory_id="dst-traj",
                target_tenant_id="demo",
                seq=0,
                source_nodes=src.list_nodes("src-traj", tenant_id="demo"),
            )
        assert dst.list_nodes("dst-traj", tenant_id="demo") == []
    finally:
        src.close()
        dst.close()
