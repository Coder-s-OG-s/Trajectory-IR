"""trajir-pkg-sig-v1 Python parity tests (Phase C / #179)."""

from __future__ import annotations

import base64
import hashlib
import json
from pathlib import Path

import pytest
from nacl.signing import SigningKey, VerifyKey

from trajectory_ir.package.errors import TirError
from trajectory_ir.package.signature import (
    TirSignatureError,
    domain_separated_message,
    key_id,
    payload_hash_from_members,
    sign_package,
    verify_package,
)
from trajectory_ir.package.tir import export_tir, load_tir
from trajectory_ir.runtime.log import NodeLog


def test_tir_signature_error_is_tir_error():
    assert issubclass(TirSignatureError, TirError)
    caught = False
    try:
        raise TirSignatureError("boom")
    except TirError:
        caught = True
    assert caught


_GOLDEN = (
    Path(__file__).resolve().parents[2]
    / "go"
    / "trajir"
    / "tir"
    / "testdata"
    / "sig_v1"
    / "payload_golden.json"
)


def _test_only_private_key() -> bytes:
    """Same seed as Go testOnlySeed: SHA-256 of the fixed note string."""
    seed = hashlib.sha256(b"trajir-pkg-sig-v1-test-vector-seed").digest()
    sk = SigningKey(seed)
    # Go ed25519.PrivateKey is seed || public (64 bytes).
    return seed + bytes(sk.verify_key)


def test_payload_and_domain_match_go_golden():
    golden = json.loads(_GOLDEN.read_text(encoding="utf-8"))
    members = {name: base64.b64decode(b64) for name, b64 in golden["members_b64"].items()}
    payload = payload_hash_from_members(members)
    assert payload.hex() == golden["payload_hash_hex"]
    msg = domain_separated_message(payload)
    assert msg.hex() == golden["domain_message_hex"]
    pub = base64.b64decode(golden["public_key_b64"])
    assert key_id(pub) == golden["key_id"]


def test_verify_go_golden_signature():
    golden = json.loads(_GOLDEN.read_text(encoding="utf-8"))
    members = {name: base64.b64decode(b64) for name, b64 in golden["members_b64"].items()}
    payload = payload_hash_from_members(members)
    msg = domain_separated_message(payload)
    pub = base64.b64decode(golden["public_key_b64"])
    sig = base64.b64decode(golden["signature_b64"])
    VerifyKey(pub).verify(msg, sig)


def test_sign_and_verify_roundtrip(tmp_path):
    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "p.tir"
    export_tir(log, "t1", out, mode="thin")
    key = _test_only_private_key()
    sign_package(out, key)
    info = verify_package(out)
    assert info is not None
    assert info.key_id == key_id(key[32:])
    pkg = load_tir(out)
    assert pkg.signature is not None


def test_export_sign_key_roundtrip(tmp_path):
    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "signed.tir"
    key = _test_only_private_key()
    export_tir(log, "t1", out, mode="thin", sign_key=key)
    info = verify_package(out)
    assert info is not None


def test_export_rejects_short_sign_key(tmp_path):
    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "bad.tir"
    with pytest.raises(TirSignatureError):
        export_tir(log, "t1", out, mode="thin", sign_key=b"\x00" * 32)
    assert not out.exists()


def test_require_signature_rejects_unsigned(tmp_path):
    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "u.tir"
    export_tir(log, "t1", out, mode="thin")
    with pytest.raises(TirSignatureError):
        verify_package(out, require_signature=True)
    assert verify_package(out) is None


def test_tamper_fails_verify(tmp_path):
    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "t.tir"
    key = _test_only_private_key()
    export_tir(log, "t1", out, mode="thin", sign_key=key)
    # Rebuild zip with mutated nodes but keep the old SIGNATURE bytes.
    import zipfile

    with zipfile.ZipFile(out, "r") as zf:
        members = {n: zf.read(n) for n in zf.namelist() if not n.endswith("/")}
    sig = members.pop("SIGNATURE")
    members["nodes.ndjson"] = b'{"id":"tampered"}\n'
    rebuild = tmp_path / "tampered.tir"
    with zipfile.ZipFile(rebuild, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for name, data in members.items():
            zf.writestr(name, data)
        zf.writestr("SIGNATURE", sig)
    with pytest.raises(TirSignatureError):
        verify_package(rebuild)


def test_verify_rejects_oversized_member(tmp_path, monkeypatch):
    """Signature path must enforce the same zip budgets as tir.load."""
    import zipfile

    import trajectory_ir.package.tir as tirmod
    from trajectory_ir.package.tir import TirLimitError

    bomb = tmp_path / "bomb.tir"
    with zipfile.ZipFile(bomb, "w", compression=zipfile.ZIP_STORED) as zf:
        zf.writestr("manifest.json", b"{}")
        zf.writestr("big.bin", b"x" * 1024)

    # Shrink limits so a tiny fixture trips the same TirLimitError path.
    monkeypatch.setattr(tirmod, "MAX_UNCOMPRESSED_ENTRY_BYTES", 100)
    monkeypatch.setattr(tirmod, "MAX_TOTAL_UNCOMPRESSED_BYTES", 100)
    with pytest.raises(TirLimitError):
        verify_package(bomb)


def test_collect_zip_members_uses_bounded_read(tmp_path, monkeypatch):
    """Member bytes must be read with an explicit cap (lying headers cannot OOM)."""
    import zipfile

    import trajectory_ir.package.tir as tirmod
    from trajectory_ir.package.signature import _collect_zip_members

    path = tmp_path / "bounded.tir"
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_STORED) as zf:
        zf.writestr("manifest.json", b"{}")
        zf.writestr("a.bin", b"abcd")

    monkeypatch.setattr(tirmod, "MAX_UNCOMPRESSED_ENTRY_BYTES", 100)
    monkeypatch.setattr(tirmod, "MAX_TOTAL_UNCOMPRESSED_BYTES", 10_000)

    read_sizes: list[int] = []
    real_open = zipfile.ZipFile.open

    def tracking_open(self, name, *args, **kwargs):
        fp = real_open(self, name, *args, **kwargs)
        real_read = fp.read

        def capped_read(n=-1):
            read_sizes.append(n)
            return real_read(n)

        fp.read = capped_read  # type: ignore[method-assign]
        return fp

    monkeypatch.setattr(zipfile.ZipFile, "open", tracking_open)
    with zipfile.ZipFile(path, "r") as zf:
        members, sig = _collect_zip_members(zf)
    assert sig is None
    assert members["a.bin"] == b"abcd"
    assert read_sizes
    assert all(n == 101 for n in read_sizes)


@pytest.mark.parametrize("body", [b"null", b"42", b"[]", b'"x"'])
def test_verify_rejects_non_object_signature_json(tmp_path, body):
    import zipfile

    from trajectory_ir.package.signature import verify_package_from_zip

    path = tmp_path / "bad.tir"
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("manifest.json", b"{}")
        zf.writestr("SIGNATURE", body)
    with (
        zipfile.ZipFile(path, "r") as zf,
        pytest.raises(TirSignatureError, match="must be an object"),
    ):
        verify_package_from_zip(zf)


def test_load_verifies_from_open_archive(tmp_path, monkeypatch):
    """load_tir must verify against the same open zip, not re open the path."""
    import zipfile

    import trajectory_ir.package.tir as tirmod

    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "same.tir"
    key = _test_only_private_key()
    export_tir(log, "t1", out, mode="thin", sign_key=key)

    opened: list[zipfile.ZipFile] = []
    real = tirmod.verify_package_from_zip

    def _track(zf, **kwargs):
        opened.append(zf)
        return real(zf, **kwargs)

    monkeypatch.setattr(tirmod, "verify_package_from_zip", _track)
    pkg = load_tir(out)
    assert pkg.signature is not None
    assert len(opened) == 1


def test_load_unsigned_skips_signature_pass(tmp_path, monkeypatch):
    """Unsigned packages must not pay for a second decompress (match Go)."""
    import trajectory_ir.package.tir as tirmod

    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "unsigned.tir"
    export_tir(log, "t1", out, mode="thin")

    calls = {"n": 0}
    real = tirmod.verify_package_from_zip

    def _count(zf, **kwargs):
        calls["n"] += 1
        return real(zf, **kwargs)

    monkeypatch.setattr(tirmod, "verify_package_from_zip", _count)
    pkg = load_tir(out)
    assert pkg.signature is None
    assert calls["n"] == 0


def test_sign_rejects_naive_signed_at(tmp_path):
    from datetime import datetime

    from trajectory_ir.package.signature import SignerMeta

    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "naive.tir"
    export_tir(log, "t1", out, mode="thin")
    key = _test_only_private_key()
    with pytest.raises(TirSignatureError, match="timezone aware"):
        sign_package(out, key, SignerMeta(signed_at=datetime(2026, 1, 1, 12, 0, 0)))


def test_export_sign_failure_removes_unsigned_file(tmp_path, monkeypatch):
    import trajectory_ir.package.tir as tirmod

    log = NodeLog(tmp_path / "nodes.sqlite")
    log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t1", "demo", 1)
    out = tmp_path / "gone.tir"
    key = _test_only_private_key()

    def _boom(*_a, **_k):
        raise OSError("disk full")

    monkeypatch.setattr(tirmod, "sign_package", _boom)
    with pytest.raises(OSError, match="disk full"):
        export_tir(log, "t1", out, mode="thin", sign_key=key)
    assert not out.exists()
