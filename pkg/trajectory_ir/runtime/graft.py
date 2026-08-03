"""Graft artifact references between trajectories (README §10 R07).

Transfers only artifact identity (content hash / ref metadata). Never copies
private THOUGHT nodes (or other private kinds) from source history.
"""

from __future__ import annotations

from typing import Any

from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import Node

# Node kinds that must never cross a graft boundary.
PRIVATE_KINDS = frozenset({"THOUGHT"})

ARTIFACT_KINDS = frozenset({"ARTIFACT_PUT", "ARTIFACT_REF"})


class GraftError(Exception):
    """Raised when a graft request is invalid or would leak private data."""


def _payload_content_hash(payload: dict[str, Any]) -> str | None:
    for key in ("content_hash", "hash", "sha256"):
        v = payload.get(key)
        if isinstance(v, str) and v:
            return v
    return None


def find_artifact_ref(
    source_nodes: list[dict[str, Any]],
    content_hash: str,
) -> dict[str, Any]:
    """Locate ARTIFACT_PUT/REF metadata for content_hash; ignore private kinds."""
    if not content_hash or not isinstance(content_hash, str):
        raise GraftError("content_hash is required")
    # Reject accidental use of a thought id or non-hash token without search.
    matches: list[dict[str, Any]] = []
    for n in source_nodes:
        kind = n.get("kind")
        if kind in PRIVATE_KINDS:
            continue
        if kind not in ARTIFACT_KINDS:
            continue
        payload = n.get("payload") or {}
        if not isinstance(payload, dict):
            continue
        if _payload_content_hash(payload) == content_hash:
            matches.append(n)
    if not matches:
        raise GraftError(f"no ARTIFACT_PUT/ARTIFACT_REF with content_hash={content_hash!r}")
    # Prefer ARTIFACT_REF then PUT; stable by seq.
    matches.sort(key=lambda n: (0 if n.get("kind") == "ARTIFACT_REF" else 1, n.get("seq") or 0))
    return matches[0]


def graft_artifact_ref(
    target_log: NodeLog,
    *,
    content_hash: str,
    target_trajectory_id: str,
    target_tenant_id: str,
    seq: int,
    step_n: int | None = None,
    source_nodes: list[dict[str, Any]] | None = None,
    logical_path: str | None = None,
    uri: str | None = None,
    source_trajectory_id: str | None = None,
) -> Node:
    """Append an ARTIFACT_REF on the target log for ``content_hash``.

    If ``source_nodes`` is provided, metadata is taken from the first matching
    artifact node (never from THOUGHT). Private kinds are never copied as nodes.
    """
    if source_nodes is not None:
        # Fail closed if caller tries to pass a private-kind node as the only "match".
        for n in source_nodes:
            if n.get("kind") in PRIVATE_KINDS:
                ph = n.get("payload") if isinstance(n.get("payload"), dict) else {}
                if isinstance(ph, dict) and _payload_content_hash(ph) == content_hash:
                    raise GraftError("refusing to graft from a THOUGHT/private node")
        src = find_artifact_ref(source_nodes, content_hash)
        payload_src = src.get("payload") or {}
        if not isinstance(payload_src, dict):
            payload_src = {}
        logical_path = logical_path or payload_src.get("logical_path")  # type: ignore[assignment]
        uri = uri if uri is not None else payload_src.get("uri")  # type: ignore[assignment]
        if source_trajectory_id is None:
            source_trajectory_id = src.get("trajectory_id")  # type: ignore[assignment]

    payload: dict[str, Any] = {
        "content_hash": content_hash,
        "grafted": True,
    }
    if logical_path is not None:
        payload["logical_path"] = logical_path
    if uri is not None:
        payload["uri"] = uri
    if source_trajectory_id is not None:
        payload["source_trajectory_id"] = source_trajectory_id

    return target_log.append(
        "ARTIFACT_REF",
        step_n,
        payload,
        target_trajectory_id,
        target_tenant_id,
        seq,
    )
