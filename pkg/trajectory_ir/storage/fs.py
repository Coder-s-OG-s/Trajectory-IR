"""Local filesystem CAS for the ``local`` deployment profile.

Root directory layout (README section 11.2)::

    <root>/
      cas/
        e3/
          e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
        ab/
          abcd...

Writes use a temp file in the same directory plus ``os.replace`` so readers
never observe a partial object. Hash is always recomputed on ``get``.
"""

from __future__ import annotations

import contextlib
import os
import tempfile
from pathlib import Path

from trajectory_ir.storage.cas import (
    CASIntegrityError,
    CASNotFoundError,
    content_hash,
    normalize_content_hash,
    shard_key,
)


class FileSystemCAS:
    """Filesystem backed content addressed store.

    Args:
        root: Directory that will contain the ``cas/`` tree. Created if missing.
    """

    def __init__(self, root: str | Path) -> None:
        self._root = Path(root).resolve()
        self._root.mkdir(parents=True, exist_ok=True)

    @property
    def root(self) -> Path:
        """Absolute root directory for this store."""
        return self._root

    def path_for(self, content_hash_hex: str) -> Path:
        """Absolute path for a content hash under this store's root."""
        return self._root / shard_key(content_hash_hex)

    def uri_for(self, content_hash_hex: str) -> str:
        """Stable ``cas://`` URI for thin package artifact refs.

        Format: ``cas://fs/<absolute-root-as-posix>/<shard>/<hash>`` is avoided;
        we use ``cas://<hash>`` plus store context at rehydrate time so packages
        stay portable across machines. Callers that need a file URL can use
        ``path_for(...).as_uri()`` locally.
        """
        h = normalize_content_hash(content_hash_hex)
        return f"cas://{h}"

    def put(self, data: bytes) -> str:
        if not isinstance(data, (bytes, bytearray)):
            raise TypeError("put expects bytes")
        data = bytes(data)
        h = content_hash(data)
        dest = self.path_for(h)
        if dest.is_file():
            existing = dest.read_bytes()
            if content_hash(existing) != h:
                raise CASIntegrityError(
                    f"corrupt object at {dest}: stored bytes do not match name {h}"
                )
            # Idempotent: same hash already present.
            return h

        dest.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp_name = tempfile.mkstemp(prefix=f".{h}.", dir=str(dest.parent))
        try:
            with os.fdopen(fd, "wb") as tmp:
                tmp.write(data)
                tmp.flush()
                os.fsync(tmp.fileno())
            os.replace(tmp_name, dest)
        except Exception:
            with contextlib.suppress(OSError):
                os.unlink(tmp_name)
            raise
        return h

    def get(self, content_hash_hex: str) -> bytes:
        h = normalize_content_hash(content_hash_hex)
        path = self.path_for(h)
        if not path.is_file():
            raise CASNotFoundError(f"no object for content_hash={h}")
        data = path.read_bytes()
        actual = content_hash(data)
        if actual != h:
            raise CASIntegrityError(
                f"object at {path} failed hash verify: expected {h}, got {actual}"
            )
        return data

    def has(self, content_hash_hex: str) -> bool:
        h = normalize_content_hash(content_hash_hex)
        return self.path_for(h).is_file()
