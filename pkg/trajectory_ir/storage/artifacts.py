"""Helpers that connect tool output bytes to CAS and ArtifactRef.

Typical host pattern::

    store = FileSystemCAS(root)
    ref = put_artifact(store, data, logical_path="out.bin")
    export_tir(log, traj_id, "run.tir", mode="thin", artifacts=[ref])
    # later, on another machine with the same CAS root or an S3 store:
    pkg = load_tir("run.tir")
    bytes_by_hash = rehydrate_artifacts(store, pkg.artifacts_manifest)
"""

from __future__ import annotations

from typing import Any

from trajectory_ir.package.tir import ArtifactRef
from trajectory_ir.storage.cas import (
    CAS,
    CASError,
    CASNotFoundError,
    content_hash,
    normalize_content_hash,
)


def put_artifact(
    store: CAS,
    data: bytes,
    *,
    logical_path: str,
    uri: str | None = None,
) -> ArtifactRef:
    """Store ``data`` in ``store`` and return a thin package artifact ref.

    Args:
        store: Any CAS implementation (filesystem, S3, ...).
        data: Raw artifact bytes.
        logical_path: Path shown in the package manifest (not used as a store key).
        uri: Optional override. When omitted, uses ``store.uri_for(hash)`` if the
            store exposes that method, otherwise ``cas://<hash>``.

    Returns:
        ArtifactRef ready for ``export_tir(..., mode="thin", artifacts=[...])``.
    """
    if not logical_path or not isinstance(logical_path, str):
        raise ValueError("logical_path is required")
    if not isinstance(data, (bytes, bytearray)):
        raise TypeError("data must be bytes")
    data_b = bytes(data)
    h = store.put(data_b)
    if content_hash(data_b) != h:
        raise RuntimeError(f"CAS put returned hash {h} that does not match content")
    if uri is None:
        uri_fn = getattr(store, "uri_for", None)
        uri = str(uri_fn(h)) if callable(uri_fn) else f"cas://{h}"
    return ArtifactRef(
        logical_path=logical_path,
        content_hash=h,
        uri=uri,
        size=len(data_b),
    )


def ensure_artifacts_in_cas(
    store: CAS,
    artifacts_manifest: list[dict[str, Any]],
    *,
    require_all: bool = True,
) -> None:
    """Fail closed if thin package hashes are missing from ``store``.

    Used by ``export_tir`` / ``import_tir`` when a CAS is supplied so packages
    do not claim rehydratable artifacts that the store cannot serve.
    """
    missing: list[str] = []
    for entry in artifacts_manifest:
        raw = entry.get("content_hash")
        if raw is None:
            raise CASError("artifact manifest entry missing content_hash")
        h = normalize_content_hash(str(raw))
        if not store.has(h):
            missing.append(h)
    if missing and require_all:
        raise CASNotFoundError(
            "CAS missing content hashes required by package: " + ", ".join(missing)
        )
