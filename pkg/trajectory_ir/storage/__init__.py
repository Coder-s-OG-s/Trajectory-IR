"""Content addressed artifact storage (CAS).

README section 11 splits durable storage into the IR node/seal log and a
content addressed object store for artifact bytes. This package owns the
object store contract and the local filesystem implementation used by the
``local`` deployment profile.

S3 and other remote drivers live under ``drivers/`` and implement the same
:class:`~trajectory_ir.storage.cas.CAS` protocol so thin ``.tir`` packages can
rehydrate from any store that verifies hashes on put and get.
"""

from trajectory_ir.storage.cas import (
    CAS,
    CASError,
    CASIntegrityError,
    CASNotFoundError,
    content_hash,
    shard_key,
)
from trajectory_ir.storage.fs import FileSystemCAS
from trajectory_ir.storage.rehydrate import rehydrate_artifacts

__all__ = [
    "CAS",
    "CASError",
    "CASIntegrityError",
    "CASNotFoundError",
    "FileSystemCAS",
    "content_hash",
    "rehydrate_artifacts",
    "shard_key",
]
