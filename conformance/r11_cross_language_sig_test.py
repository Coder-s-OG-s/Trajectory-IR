"""Conformance R11: Go golden signature verifies in Python.

The other direction (Python-signed package verifies in Go) is
`TestVerifyPythonSignedFixture` in go/trajir/tir, using testdata/sample_signed.tir
from scripts/gen_tir_fixture.py.
"""

from __future__ import annotations

import base64
import json
import zipfile
from pathlib import Path

from trajectory_ir.package import verify_package

_GOLDEN = (
    Path(__file__).resolve().parents[1]
    / "go"
    / "trajir"
    / "tir"
    / "testdata"
    / "sig_v1"
    / "payload_golden.json"
)


def test_r11_go_golden_verifies_in_python(tmp_path: Path) -> None:
    golden = json.loads(_GOLDEN.read_text(encoding="utf-8"))
    members = {name: base64.b64decode(b64) for name, b64 in golden["members_b64"].items()}
    doc = {
        "scheme": "trajir-pkg-sig-v1",
        "payload_alg": "trajir-pkg-payload-v1",
        "payload_hash": golden["payload_hash_hex"],
        "alg": "ed25519",
        "public_key": golden["public_key_b64"],
        "signature": golden["signature_b64"],
        "signed_at": "2026-08-13T12:00:00Z",
        "signer": {"key_id": golden["key_id"]},
        "extensions": {},
    }
    out = tmp_path / "go-golden.tir"
    with zipfile.ZipFile(out, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for name, data in members.items():
            zf.writestr(name, data)
        zf.writestr("SIGNATURE", json.dumps(doc, indent=2) + "\n")

    info = verify_package(out)
    assert info is not None
    assert info.key_id == golden["key_id"]
    assert info.payload_hash.hex() == golden["payload_hash_hex"]
