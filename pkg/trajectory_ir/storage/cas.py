"""CAS protocol and shared key layout helpers (README section 11.2).

Layout (stable across filesystem and S3 drivers)::

    cas/<first-2-hex-chars-of-sha256>/<full-sha256-hex>

Objects are identified solely by the SHA-256 of their bytes. A given hash
never changes once written; the only failure mode is a miss or a bit flip
detected by re-hashing on read.
"""

from __future__ import annotations

import hashlib
import re
from typing import Protocol, runtime_checkable

_HEX64 = re.compile(r"^[0-9a-f]{64}$")


class CASError(Exception):
    """Base error for content addressed storage operations."""


class CASNotFoundError(CASError):
    """Raised when ``get`` cannot find an object for the given content hash."""


class CASIntegrityError(CASError):
    """Raised when stored bytes do not match the expected content hash."""


def content_hash(data: bytes) -> str:
    """Return lowercase hex SHA-256 of ``data``."""
    if not isinstance(data, (bytes, bytearray)):
        raise TypeError("content_hash expects bytes")
    return hashlib.sha256(data).hexdigest()


def normalize_content_hash(value: str) -> str:
    """Validate and normalize a content hash to lowercase hex."""
    if not isinstance(value, str):
        raise TypeError("content_hash must be a string")
    h = value.lower().strip()
    if not _HEX64.match(h):
        raise CASError(f"invalid content hash (need 64 lowercase hex digits): {value!r}")
    return h


def shard_key(content_hash_hex: str) -> str:
    """Return the relative object key for a content hash (no scheme, no root).

    Example: ``e3b0c442...`` -> ``cas/e3/e3b0c442...``
    """
    h = normalize_content_hash(content_hash_hex)
    return f"cas/{h[:2]}/{h}"


@runtime_checkable
class CAS(Protocol):
    """Minimal content addressed store used by thin package rehydrate.

    Implementations must:
    1. Compute identity as SHA-256 of the bytes (hex).
    2. Store under the sharded key layout from :func:`shard_key`.
    3. Verify the hash on every successful ``get``.
    4. Treat concurrent ``put`` of the same bytes as idempotent.
    """

    def put(self, data: bytes) -> str:
        """Store ``data`` and return its content hash.

        If the object already exists with matching bytes, return the hash
        without rewriting. If a different payload occupies the hash path,
        raise :class:`CASIntegrityError` (should be impossible for true CAS).
        """
        ...

    def get(self, content_hash_hex: str) -> bytes:
        """Load bytes for ``content_hash_hex``, verifying the hash.

        Raises:
            CASNotFoundError: object is absent
            CASIntegrityError: bytes present but hash does not match
        """
        ...

    def has(self, content_hash_hex: str) -> bool:
        """Return True if an object exists for this hash (no full verify)."""
        ...
