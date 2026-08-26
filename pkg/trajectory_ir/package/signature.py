"""Package signatures for .tir (README 9.1, trajir-pkg-sig-v1).

File only SIGNATURE zip member; manifest.signature stays null.
Payload: trajir-pkg-payload-v1 over all members except SIGNATURE.
Crypto: domain separated Ed25519 (byte identical to go/trajir/tir).
"""

from __future__ import annotations

import base64
import contextlib
import hashlib
import json
import os
import struct
import tempfile
import zipfile
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from nacl.exceptions import BadSignatureError
from nacl.signing import SigningKey, VerifyKey

SIGNATURE_MEMBER = "SIGNATURE"
SCHEME_V1 = "trajir-pkg-sig-v1"
PAYLOAD_ALG_V1 = "trajir-pkg-payload-v1"
ALG_ED25519 = "ed25519"

# Match Go ed25519.PrivateKeySize / PublicKeySize.
_PRIVATE_KEY_SIZE = 64
_SEED_SIZE = 32
_PUBLIC_KEY_SIZE = 32


class TirSignatureError(Exception):
    """Package signature failure (missing when required, tamper, unknown scheme)."""


@dataclass(frozen=True)
class SignerMeta:
    """Optional metadata written into the SIGNATURE document."""

    id: str = ""
    signed_at: datetime | None = None


@dataclass(frozen=True)
class SignatureInfo:
    """Result of a successful verify."""

    document: dict[str, Any]
    payload_hash: bytes
    public_key: bytes
    key_id: str
    signer_id: str


def key_id(public_key: bytes) -> str:
    """First 16 hex chars of SHA-256(public_key raw)."""
    if len(public_key) != _PUBLIC_KEY_SIZE:
        raise TirSignatureError("invalid public key size")
    return hashlib.sha256(public_key).hexdigest()[:16]


def domain_separated_message(payload_hash: bytes) -> bytes:
    """SHA-256(scheme || 0x00 || payload_hash_raw)."""
    if len(payload_hash) != 32:
        raise TirSignatureError("payload_hash must be 32 bytes")
    h = hashlib.sha256()
    h.update(SCHEME_V1.encode("ascii"))
    h.update(b"\x00")
    h.update(payload_hash)
    return h.digest()


def payload_hash_from_members(members: dict[str, bytes]) -> bytes:
    """trajir-pkg-payload-v1 over members (must not include SIGNATURE)."""
    names = sorted(n for n in members if n != SIGNATURE_MEMBER)
    stream = hashlib.sha256()
    for name in names:
        _safe_member_name(name)
        name_b = name.encode("utf-8")
        stream.update(struct.pack(">Q", len(name_b)))
        stream.update(name_b)
        stream.update(hashlib.sha256(members[name]).digest())
    return stream.digest()


def _safe_member_name(name: str) -> None:
    # Match tir._safe_zip_name / Go safeZipName so load and sign agree.
    if not name or name.startswith(("/", "\\")):
        raise TirSignatureError(f"unsafe zip path: {name!r}")
    parts = name.replace("\\", "/").split("/")
    if parts and parts[0] == "":
        raise TirSignatureError(f"unsafe zip path: {name!r}")
    if ".." in parts:
        raise TirSignatureError(f"unsafe zip path: {name!r}")
    if name.startswith("artifacts/") and not (
        name == "artifacts/" or name.startswith("artifacts/cas/")
    ):
        raise TirSignatureError(f"unexpected artifact path: {name!r}")


def _signing_key_from_bytes(key: bytes) -> SigningKey:
    # Match Go Sign: public API accepts only the full 64 byte private key.
    if len(key) != _PRIVATE_KEY_SIZE:
        raise TirSignatureError("invalid ed25519 private key size")
    return SigningKey(key[:_SEED_SIZE])


def _collect_zip_members(
    zf: zipfile.ZipFile,
) -> tuple[dict[str, bytes], bytes | None]:
    """Read members from an already open archive with tir.py size budgets."""
    # Local import avoids a cycle: tir.py imports this module at load time.
    from trajectory_ir.package.tir import (
        MAX_TOTAL_UNCOMPRESSED_BYTES,
        MAX_UNCOMPRESSED_ENTRY_BYTES,
        MAX_ZIP_ENTRIES,
        TirLimitError,
    )

    members: dict[str, bytes] = {}
    signature: bytes | None = None
    infos = [i for i in zf.infolist() if not i.filename.endswith("/")]
    if len(infos) > MAX_ZIP_ENTRIES:
        raise TirLimitError(f"too many zip entries: {len(infos)} > {MAX_ZIP_ENTRIES}")
    budget = MAX_TOTAL_UNCOMPRESSED_BYTES
    for info in infos:
        name = info.filename
        _safe_member_name(name)
        # Declared size is advisory only: a malicious header can understate
        # file_size. Cap the read so a lying local header cannot OOM us.
        if info.file_size > MAX_UNCOMPRESSED_ENTRY_BYTES:
            raise TirLimitError(
                f"zip member {name!r} uncompressed size {info.file_size} "
                f"exceeds limit {MAX_UNCOMPRESSED_ENTRY_BYTES}"
            )
        limit = min(MAX_UNCOMPRESSED_ENTRY_BYTES, budget)
        with zf.open(name) as fp:
            data = fp.read(limit + 1)
        if len(data) > MAX_UNCOMPRESSED_ENTRY_BYTES:
            raise TirLimitError(f"zip member {name!r} expanded beyond per-entry limit")
        if len(data) > budget:
            raise TirLimitError(
                f"zip total uncompressed size would exceed limit {MAX_TOTAL_UNCOMPRESSED_BYTES}"
            )
        budget -= len(data)
        if name == SIGNATURE_MEMBER:
            if signature is not None:
                raise TirSignatureError("duplicate SIGNATURE member")
            signature = data
            continue
        if name in members:
            raise TirSignatureError(f"duplicate zip member {name!r}")
        members[name] = data
    return members, signature


def _read_zip_members(path: Path) -> tuple[dict[str, bytes], bytes | None]:
    with zipfile.ZipFile(path, "r") as zf:
        return _collect_zip_members(zf)


def verify_package_from_zip(
    zf: zipfile.ZipFile,
    *,
    require_signature: bool = False,
    trusted_keys: list[bytes] | None = None,
    trusted_key_ids: list[str] | None = None,
) -> SignatureInfo | None:
    """Verify SIGNATURE against members from an already open ZipFile.

    Prefer this from load_tir so the archive is never re opened (TOCTOU).
    """
    members, sig_raw = _collect_zip_members(zf)
    if sig_raw is None:
        if require_signature:
            raise TirSignatureError("package is unsigned (SIGNATURE missing)")
        return None
    return _verify_signature_bytes(
        members,
        sig_raw,
        trusted_keys=trusted_keys,
        trusted_key_ids=trusted_key_ids,
    )


def verify_package(
    path: str | Path,
    *,
    require_signature: bool = False,
    trusted_keys: list[bytes] | None = None,
    trusted_key_ids: list[str] | None = None,
) -> SignatureInfo | None:
    """Verify SIGNATURE on a .tir package.

    Returns None when unsigned and require_signature is False.
    """
    path = Path(path)
    with zipfile.ZipFile(path, "r") as zf:
        return verify_package_from_zip(
            zf,
            require_signature=require_signature,
            trusted_keys=trusted_keys,
            trusted_key_ids=trusted_key_ids,
        )


def _verify_signature_bytes(
    members: dict[str, bytes],
    sig_raw: bytes,
    *,
    trusted_keys: list[bytes] | None,
    trusted_key_ids: list[str] | None,
) -> SignatureInfo:
    try:
        doc = json.loads(sig_raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as e:
        raise TirSignatureError(f"SIGNATURE JSON: {e}") from e
    if not isinstance(doc, dict):
        raise TirSignatureError("SIGNATURE JSON must be an object")
    if doc.get("scheme") != SCHEME_V1:
        raise TirSignatureError(f"unknown scheme {doc.get('scheme')!r}")
    if doc.get("payload_alg") != PAYLOAD_ALG_V1:
        raise TirSignatureError(f"unknown payload_alg {doc.get('payload_alg')!r}")
    if doc.get("alg") != ALG_ED25519:
        raise TirSignatureError(f"unknown alg {doc.get('alg')!r}")

    want_hash = payload_hash_from_members(members)
    got_hex = str(doc.get("payload_hash", "")).strip().lower()
    if got_hex != want_hash.hex():
        raise TirSignatureError("payload_hash mismatch")

    try:
        pub_raw = base64.b64decode(doc["public_key"], validate=True)
    except Exception as e:
        raise TirSignatureError("invalid public_key") from e
    if len(pub_raw) != _PUBLIC_KEY_SIZE:
        raise TirSignatureError("invalid public_key")

    try:
        sig = base64.b64decode(doc["signature"], validate=True)
    except Exception as e:
        raise TirSignatureError("invalid signature encoding") from e
    if len(sig) != 64:
        raise TirSignatureError("invalid signature encoding")

    msg = domain_separated_message(want_hash)
    try:
        VerifyKey(pub_raw).verify(msg, sig)
    except BadSignatureError as e:
        raise TirSignatureError("ed25519 verification failed") from e

    signer = doc.get("signer") or {}
    if not isinstance(signer, dict):
        raise TirSignatureError("signer must be an object")
    kid = str(signer.get("key_id") or "") or key_id(pub_raw)
    if signer.get("key_id") and kid != key_id(pub_raw):
        raise TirSignatureError("signer.key_id does not match public_key")

    if trusted_keys and not any(k == pub_raw for k in trusted_keys):
        raise TirSignatureError("public key not in trust store")
    if trusted_key_ids and kid not in trusted_key_ids:
        raise TirSignatureError(f"key_id {kid!r} not in trust store")

    return SignatureInfo(
        document=doc,
        payload_hash=want_hash,
        public_key=pub_raw,
        key_id=kid,
        signer_id=str(signer.get("id") or ""),
    )


def sign_package(
    path: str | Path,
    private_key: bytes,
    meta: SignerMeta | None = None,
) -> None:
    """Write or replace the SIGNATURE member of an existing .tir package."""
    # Match Go Sign: require full 64 byte private key for the public API.
    if len(private_key) != _PRIVATE_KEY_SIZE:
        raise TirSignatureError("invalid ed25519 private key size")

    path = Path(path)
    members, _existing = _read_zip_members(path)
    payload = payload_hash_from_members(members)
    sk = _signing_key_from_bytes(private_key)
    pub = bytes(sk.verify_key)
    msg = domain_separated_message(payload)
    sig = sk.sign(msg).signature

    meta = meta or SignerMeta()
    signed_at = meta.signed_at or datetime.now(UTC)
    if signed_at.tzinfo is None:
        raise TirSignatureError("signed_at must be timezone aware")
    signer: dict[str, str] = {"key_id": key_id(pub)}
    if meta.id:
        signer["id"] = meta.id
    doc = {
        "scheme": SCHEME_V1,
        "payload_alg": PAYLOAD_ALG_V1,
        "payload_hash": payload.hex(),
        "alg": ALG_ED25519,
        "public_key": base64.b64encode(pub).decode("ascii"),
        "signature": base64.b64encode(sig).decode("ascii"),
        "signed_at": signed_at.astimezone(UTC).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "signer": signer,
        "extensions": {},
    }
    doc_bytes = (json.dumps(doc, indent=2) + "\n").encode("utf-8")
    _rewrite_zip_with_signature(path, members, doc_bytes)


def _rewrite_zip_with_signature(
    path: Path, members: dict[str, bytes], signature_json: bytes
) -> None:
    orig_mode = path.stat().st_mode & 0o777
    parent = path.parent
    fd, tmp_name = tempfile.mkstemp(prefix="tir-sign-", suffix=".tmp", dir=str(parent))
    try:
        with os.fdopen(fd, "wb") as raw:
            with zipfile.ZipFile(raw, "w", compression=zipfile.ZIP_DEFLATED) as zf:
                for name in sorted(members):
                    zf.writestr(name, members[name])
                zf.writestr(SIGNATURE_MEMBER, signature_json)
            raw.flush()
            os.fsync(raw.fileno())
        os.chmod(tmp_name, orig_mode)
        os.replace(tmp_name, path)
    except Exception:
        with contextlib.suppress(OSError):
            os.unlink(tmp_name)
        raise
