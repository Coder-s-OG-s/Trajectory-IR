"""Conformance R09: sign a thin package, verify, then tamper must fail."""

from __future__ import annotations

import hashlib
import zipfile
from pathlib import Path

from nacl.signing import SigningKey

from trajectory_ir.package import TirSignatureError, export_tir, sign_package, verify_package
from trajectory_ir.runtime.log import NodeLog


def _test_only_key() -> bytes:
    seed = hashlib.sha256(b"trajir-pkg-sig-v1-test-vector-seed").digest()
    sk = SigningKey(seed)
    return seed + bytes(sk.verify_key)


def _seed(log: NodeLog, trajectory_id: str = "r09-traj") -> None:
    log.append(
        "DECISION",
        1,
        {"plan": {"tool_calls": []}},
        trajectory_id=trajectory_id,
        tenant_id="demo",
        seq=1,
    )
    log.append(
        "COMMIT_STEP",
        1,
        {},
        trajectory_id=trajectory_id,
        tenant_id="demo",
        seq=2,
    )


def test_r09_sign_verify_then_mutate_nodes_fails(tmp_path: Path) -> None:
    log = NodeLog(str(tmp_path / "src.sqlite"))
    try:
        _seed(log)
        pkg_path = tmp_path / "r09.tir"
        export_tir(log, "r09-traj", pkg_path, mode="thin")
        sign_package(pkg_path, _test_only_key())
        info = verify_package(pkg_path)
        assert info is not None
        assert info.document["scheme"] == "trajir-pkg-sig-v1"
    finally:
        log.close()

    with zipfile.ZipFile(pkg_path, "r") as zf:
        members = {n: zf.read(n) for n in zf.namelist() if not n.endswith("/")}
    sig = members.pop("SIGNATURE")
    nodes = bytearray(members["nodes.ndjson"])
    nodes[0] ^= 0x01
    members["nodes.ndjson"] = bytes(nodes)
    tampered = tmp_path / "r09-tampered.tir"
    with zipfile.ZipFile(tampered, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for name, data in members.items():
            zf.writestr(name, data)
        zf.writestr("SIGNATURE", sig)

    try:
        verify_package(tampered)
    except TirSignatureError:
        return
    raise AssertionError("tampered nodes.ndjson must fail verify")
