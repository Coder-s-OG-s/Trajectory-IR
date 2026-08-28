"""Conformance R10: unsigned packages still load; strict verify rejects them."""

from __future__ import annotations

from pathlib import Path

from trajectory_ir.package import (
    TirSignatureError,
    export_tir,
    import_tir,
    load_tir,
    verify_package,
)
from trajectory_ir.runtime.log import NodeLog


def test_r10_unsigned_roundtrip_and_strict_reject(tmp_path: Path) -> None:
    src = NodeLog(str(tmp_path / "src.sqlite"))
    try:
        src.append(
            "DECISION",
            1,
            {"plan": {"tool_calls": []}},
            trajectory_id="r10-traj",
            tenant_id="demo",
            seq=1,
        )
        pkg_path = tmp_path / "r10-unsigned.tir"
        export_tir(src, "r10-traj", pkg_path, mode="thin")
        pkg = load_tir(pkg_path)
        assert pkg.signature is None
        assert pkg.manifest.get("signature") is None
        dest = NodeLog(str(tmp_path / "dest.sqlite"))
        try:
            imported = import_tir(pkg_path, dest)
            assert imported.signature is None
            rows = dest.list_nodes("r10-traj", tenant_id="demo")
            assert len(rows) >= 1
        finally:
            dest.close()
        assert verify_package(pkg_path) is None
        try:
            verify_package(pkg_path, require_signature=True)
        except TirSignatureError:
            return
        raise AssertionError("require_signature must reject unsigned packages")
    finally:
        src.close()
