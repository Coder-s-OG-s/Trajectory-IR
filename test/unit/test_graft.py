"""Unit coverage for trajectory_ir.runtime.graft (R07)."""

from __future__ import annotations

import pytest

from trajectory_ir.runtime.graft import GraftError, find_artifact_ref, graft_artifact_ref
from trajectory_ir.runtime.log import NodeLog


def test_find_artifact_ref_prefers_ref_over_put():
    h = "ab" + "0" * 62
    nodes = [
        {
            "kind": "ARTIFACT_PUT",
            "payload": {"content_hash": h, "logical_path": "put.bin"},
            "seq": 1,
            "trajectory_id": "s",
        },
        {
            "kind": "ARTIFACT_REF",
            "payload": {"content_hash": h, "logical_path": "ref.bin"},
            "seq": 2,
            "trajectory_id": "s",
        },
        {"kind": "THOUGHT", "payload": {"content_hash": h, "text": "nope"}, "seq": 0},
    ]
    found = find_artifact_ref(nodes, h)
    assert found["kind"] == "ARTIFACT_REF"
    assert found["payload"]["logical_path"] == "ref.bin"


def test_find_artifact_ref_missing():
    with pytest.raises(GraftError, match="no ARTIFACT"):
        find_artifact_ref([{"kind": "DECISION", "payload": {}, "seq": 0}], "ff" + "1" * 62)


def test_find_artifact_ref_requires_hash():
    with pytest.raises(GraftError, match="content_hash"):
        find_artifact_ref([], "")


def test_graft_copies_metadata_not_thoughts(tmp_path):
    src = NodeLog(str(tmp_path / "src.sqlite"))
    dst = NodeLog(str(tmp_path / "dst.sqlite"))
    try:
        h = "cd" + "2" * 62
        src.append("THOUGHT", 1, {"text": "secret plan"}, "src", "demo", 0)
        src.append(
            "ARTIFACT_PUT",
            1,
            {"content_hash": h, "logical_path": "out.bin", "uri": "cas://" + h},
            "src",
            "demo",
            1,
        )
        nodes = src.list_nodes("src", tenant_id="demo")
        node = graft_artifact_ref(
            dst,
            content_hash=h,
            target_trajectory_id="dst",
            target_tenant_id="demo",
            seq=0,
            step_n=1,
            source_nodes=nodes,
        )
        assert node.kind == "ARTIFACT_REF"
        assert node.payload["content_hash"] == h
        assert node.payload["logical_path"] == "out.bin"
        assert node.payload.get("grafted") is True
        kinds = {n["kind"] for n in dst.list_nodes("dst", tenant_id="demo")}
        assert kinds == {"ARTIFACT_REF"}
    finally:
        src.close()
        dst.close()


def test_graft_refuses_thought_only_match(tmp_path):
    src = NodeLog(str(tmp_path / "src.sqlite"))
    dst = NodeLog(str(tmp_path / "dst.sqlite"))
    try:
        h = "ee" + "3" * 62
        src.append("THOUGHT", 1, {"content_hash": h, "text": "x"}, "src", "demo", 0)
        with pytest.raises(GraftError):
            graft_artifact_ref(
                dst,
                content_hash=h,
                target_trajectory_id="dst",
                target_tenant_id="demo",
                seq=0,
                source_nodes=src.list_nodes("src", tenant_id="demo"),
            )
    finally:
        src.close()
        dst.close()


def test_graft_without_source_nodes(tmp_path):
    dst = NodeLog(str(tmp_path / "dst.sqlite"))
    try:
        h = "11" + "a" * 62
        node = graft_artifact_ref(
            dst,
            content_hash=h,
            target_trajectory_id="dst",
            target_tenant_id="demo",
            seq=0,
            logical_path="manual.bin",
            uri="cas://" + h,
        )
        assert node.payload["logical_path"] == "manual.bin"
        assert node.payload["uri"] == "cas://" + h
    finally:
        dst.close()
