"""Thin package artifact rehydration against a CAS.

Fat packages embed bytes. Thin packages only carry content hashes (and
optional URIs). Call :func:`rehydrate_artifacts` after :func:`load_tir` when
you need the bytes again from a local or remote store.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from trajectory_ir.storage.cas import CAS, CASError, CASNotFoundError, normalize_content_hash


def rehydrate_artifacts(
    store: CAS,
    artifacts_manifest: list[Mapping[str, Any]],
    *,
    require_all: bool = True,
) -> dict[str, bytes]:
    """Fetch artifact bytes for each manifest entry from ``store``.

    Args:
        store: Any CAS implementation (filesystem, S3, ...).
        artifacts_manifest: Entries shaped like ``.tir`` artifacts-manifest.json
            rows (must include ``content_hash``).
        require_all: When True (default), missing objects raise
            :class:`CASNotFoundError`. When False, missing hashes are skipped.

    Returns:
        Map of content_hash -> bytes for every resolved entry.
    """
    out: dict[str, bytes] = {}
    for entry in artifacts_manifest:
        raw = entry.get("content_hash")
        if raw is None:
            raise CASError("artifact manifest entry missing content_hash")
        h = normalize_content_hash(str(raw))
        if h in out:
            continue
        try:
            out[h] = store.get(h)
        except CASNotFoundError:
            if require_all:
                raise
            continue
    return out
