"""Portable `.tir` package export and import (thin and fat modes).

Layout (master README §9):

    manifest.json
    nodes.ndjson
    seals.json
    artifacts-manifest.json
    artifacts/          # fat only
    COMPAT.json
    # SIGNATURE intentionally omitted (reserved, unimplemented)

Import verifies every node id against runtime.nodes hashing (R05 spirit).
"""

from __future__ import annotations

import hashlib
import json
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Literal

from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import Node, node_id, payload_hash

PackageMode = Literal["thin", "fat"]

PACKAGE_FORMAT_VERSION = "0.1"
COMPAT = {
    "package_format": PACKAGE_FORMAT_VERSION,
    "min_runtime": "0.1.0",
    "node_id_scheme": "sha256(tenant|trajectory|step|seq|kind|payload_hash)",
    "payload_hash_scheme": "sha256(rfc8785(payload))",
}


class TirError(Exception):
    """Base error for package operations."""


class TirVerificationError(TirError):
    """Raised when import finds a hash or structural mismatch."""


@dataclass(frozen=True)
class ArtifactRef:
    """One artifact entry for thin or fat packages."""

    logical_path: str
    content_hash: str
    uri: str | None = None  # thin: where to rehydrate from
    size: int | None = None


@dataclass
class TirPackage:
    """In-memory representation of a verified package."""

    manifest: dict[str, Any]
    nodes: list[dict[str, Any]]
    seals: list[dict[str, Any]]
    artifacts_manifest: list[dict[str, Any]]
    artifact_bytes: dict[str, bytes]  # content_hash -> bytes (fat only)


def _content_hash(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _verify_node_record(rec: dict[str, Any]) -> None:
    """Recompute id from fields; raise if it does not match the stored id."""
    payload = rec["payload"]
    if not isinstance(payload, dict):
        raise TirVerificationError(f"node {rec.get('id')}: payload must be an object")
    try:
        ph = payload_hash(payload)
    except AssertionError as e:
        raise TirVerificationError(f"node {rec.get('id')}: {e}") from e
    expected = node_id(
        rec["tenant_id"],
        rec["trajectory_id"],
        rec.get("step_n"),
        int(rec["seq"]),
        rec["kind"],
        ph,
    )
    if rec.get("id") != expected:
        raise TirVerificationError(
            f"node id mismatch for kind={rec.get('kind')} seq={rec.get('seq')}: "
            f"recorded={rec.get('id')} expected={expected}"
        )
    # Optional recorded payload_hash field
    if "payload_hash" in rec and rec["payload_hash"] != ph:
        raise TirVerificationError(
            f"payload_hash mismatch for node {rec.get('id')}: "
            f"recorded={rec['payload_hash']} expected={ph}"
        )


def _seals_from_nodes(nodes: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """DECISION rows act as decision seals in the current runtime (content-addressed append)."""
    seals: list[dict[str, Any]] = []
    for n in nodes:
        if n["kind"] == "DECISION":
            seals.append(
                {
                    "type": "DECISION_SEAL",
                    "node_id": n["id"],
                    "step_n": n.get("step_n"),
                    "seq": n["seq"],
                    "content_hash": payload_hash(n["payload"]),
                }
            )
    return seals


def export_tir(
    node_log: NodeLog,
    trajectory_id: str,
    dest: str | Path,
    *,
    mode: PackageMode = "thin",
    artifacts: list[ArtifactRef] | None = None,
    artifact_bytes: dict[str, bytes] | None = None,
) -> Path:
    """Export a trajectory from ``node_log`` to a ``.tir`` zip at ``dest``."""
    if mode not in ("thin", "fat"):
        raise TirError(f"unsupported mode {mode!r}; use thin or fat")

    nodes = node_log.list_nodes(trajectory_id)
    if not nodes:
        raise TirError(f"no nodes for trajectory_id={trajectory_id!r}")

    tenant_id = nodes[0]["tenant_id"]
    for n in nodes:
        _verify_node_record(n)

    artifacts = list(artifacts or [])
    artifact_bytes = dict(artifact_bytes or {})
    if mode == "fat":
        for ref in artifacts:
            data = artifact_bytes.get(ref.content_hash)
            if data is None:
                raise TirError(f"fat export missing bytes for content_hash={ref.content_hash}")
            if _content_hash(data) != ref.content_hash:
                raise TirError(f"artifact bytes do not match content_hash={ref.content_hash}")
    else:
        # thin: drop embedded bytes even if provided
        artifact_bytes = {}

    seals = _seals_from_nodes(nodes)
    manifest = {
        "package_format": PACKAGE_FORMAT_VERSION,
        "trajectory_id": trajectory_id,
        "tenant_id": tenant_id,
        "mode": mode,
        "node_count": len(nodes),
        "seal_count": len(seals),
        "signature": None,
    }
    artifacts_manifest = [
        {
            "logical_path": a.logical_path,
            "content_hash": a.content_hash,
            "uri": a.uri,
            "size": a.size
            if a.size is not None
            else (
                len(artifact_bytes[a.content_hash]) if a.content_hash in artifact_bytes else None
            ),
        }
        for a in artifacts
    ]

    dest_path = Path(dest)
    if dest_path.suffix != ".tir":
        dest_path = dest_path.with_suffix(".tir")

    with zipfile.ZipFile(dest_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("manifest.json", json.dumps(manifest, indent=2, sort_keys=True) + "\n")
        zf.writestr("COMPAT.json", json.dumps(COMPAT, indent=2, sort_keys=True) + "\n")
        zf.writestr("seals.json", json.dumps(seals, indent=2, sort_keys=True) + "\n")
        zf.writestr(
            "artifacts-manifest.json",
            json.dumps(artifacts_manifest, indent=2, sort_keys=True) + "\n",
        )
        ndjson = "".join(json.dumps(n, sort_keys=True) + "\n" for n in nodes)
        zf.writestr("nodes.ndjson", ndjson)
        if mode == "fat":
            for h, data in artifact_bytes.items():
                # sharded layout cas/ab/cdef... under artifacts/
                shard = h[:2]
                zf.writestr(f"artifacts/cas/{shard}/{h}", data)

    return dest_path


def load_tir(path: str | Path, *, verify: bool = True) -> TirPackage:
    """Load and optionally verify a ``.tir`` zip without writing to a NodeLog."""
    path = Path(path)
    if not path.is_file():
        raise TirError(f"package not found: {path}")

    with zipfile.ZipFile(path, "r") as zf:
        names = set(zf.namelist())
        required = {
            "manifest.json",
            "nodes.ndjson",
            "seals.json",
            "artifacts-manifest.json",
            "COMPAT.json",
        }
        missing = required - names
        if missing:
            raise TirError(f"package missing files: {sorted(missing)}")

        manifest = json.loads(zf.read("manifest.json"))
        seals = json.loads(zf.read("seals.json"))
        artifacts_manifest = json.loads(zf.read("artifacts-manifest.json"))
        nodes: list[dict[str, Any]] = []
        for line in zf.read("nodes.ndjson").decode("utf-8").splitlines():
            line = line.strip()
            if not line:
                continue
            nodes.append(json.loads(line))

        artifact_bytes: dict[str, bytes] = {}
        for name in names:
            if name.startswith("artifacts/cas/") and not name.endswith("/"):
                data = zf.read(name)
                h = Path(name).name
                if _content_hash(data) != h:
                    raise TirVerificationError(
                        f"embedded artifact name {h} does not match content hash"
                    )
                artifact_bytes[h] = data

    if verify:
        if not nodes:
            raise TirVerificationError("package has no nodes")
        for n in nodes:
            _verify_node_record(n)
        expected_seals = _seals_from_nodes(nodes)
        # Allow seals.json to be a subset or equal recomputation of DECISION seals
        by_id = {s["node_id"]: s for s in seals}
        for exp in expected_seals:
            got = by_id.get(exp["node_id"])
            if got is None:
                raise TirVerificationError(f"missing seal for decision node {exp['node_id']}")
            if got.get("content_hash") != exp["content_hash"]:
                raise TirVerificationError(f"seal content_hash mismatch for {exp['node_id']}")
        mode = manifest.get("mode", "thin")
        if mode == "fat":
            for entry in artifacts_manifest:
                h = entry["content_hash"]
                if h not in artifact_bytes:
                    raise TirVerificationError(f"fat package missing artifact bytes {h}")
        if mode == "thin" and artifact_bytes:
            # thin packages should not embed bytes
            raise TirVerificationError("thin package must not embed artifact bytes")

    return TirPackage(
        manifest=manifest,
        nodes=nodes,
        seals=seals,
        artifacts_manifest=artifacts_manifest,
        artifact_bytes=artifact_bytes,
    )


def import_tir(
    path: str | Path,
    node_log: NodeLog,
    *,
    verify: bool = True,
) -> TirPackage:
    """Verify a package and append its nodes into ``node_log`` (idempotent by id).

    Does not acquire a writer lease (README §9).
    """
    pkg = load_tir(path, verify=verify)
    for n in pkg.nodes:
        # Rebuild via Node so append stores consistent fields; id must match.
        node = Node(
            kind=n["kind"],
            trajectory_id=n["trajectory_id"],
            tenant_id=n["tenant_id"],
            step_n=n.get("step_n"),
            seq=int(n["seq"]),
            payload=n["payload"],
            ts=float(n.get("ts") or 0.0),
        )
        # Node regenerates id from payload; must equal package record.
        if node.id != n["id"]:
            raise TirVerificationError(f"rebuilt node id {node.id} != package id {n['id']}")
        node_log.append(
            node.kind,
            node.step_n,
            node.payload,
            node.trajectory_id,
            node.tenant_id,
            node.seq,
        )
    return pkg
