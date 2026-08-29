#!/usr/bin/env python3
"""Generate testdata/sample_thin.tir and testdata/sample_signed.tir."""

from __future__ import annotations

import hashlib
import sys
from pathlib import Path

from nacl.signing import SigningKey

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "pkg"))
sys.path.insert(0, str(ROOT))

from trajectory_ir.package import export_tir  # noqa: E402
from trajectory_ir.runtime.log import NodeLog  # noqa: E402


def main() -> None:
    out_dir = ROOT / "testdata"
    out_dir.mkdir(exist_ok=True)
    db = out_dir / "_fixture_nodes.sqlite"
    if db.exists():
        db.unlink()
    log = NodeLog(str(db))
    try:
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
        dest = out_dir / "sample_thin.tir"
        export_tir(log, "t-export", dest, mode="thin")
        print(f"wrote {dest}")

        seed = hashlib.sha256(b"trajir-pkg-sig-v1-test-vector-seed").digest()
        sk = SigningKey(seed)
        key = seed + bytes(sk.verify_key)
        signed = out_dir / "sample_signed.tir"
        export_tir(log, "t-export", signed, mode="thin", sign_key=key)
        print(f"wrote {signed}")
    finally:
        log.close()
        if db.exists():
            db.unlink()


if __name__ == "__main__":
    main()
