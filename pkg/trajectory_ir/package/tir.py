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

Security: load enforces zip path safety and size/count limits so untrusted
packages cannot zip-bomb or path-traverse the reader.
"""

from __future__ import annotations

import hashlib
import json
import re
import zipfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Literal

from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import Node, NodeValidationError, node_id, payload_hash

PackageMode = Literal["thin", "fat"]

PACKAGE_FORMAT_VERSION = "0.1"
COMPAT = {
    "package_format": PACKAGE_FORMAT_VERSION,
    "min_runtime": "0.1.0",
    "node_id_scheme": "sha256(tenant|trajectory|step|seq|kind|payload_hash)",
    "payload_hash_scheme": "sha256(rfc8785(payload))",
}

# Hard limits for untrusted package load (DoS / zip-bomb resistance).
MAX_ZIP_ENTRIES = 10_000
MAX_UNCOMPRESSED_ENTRY_BYTES = 32 * 1024 * 1024  # 32 MiB per member
MAX_TOTAL_UNCOMPRESSED_BYTES = 128 * 1024 * 1024  # 128 MiB total
MAX_NODES = 100_000
MAX_ARTIFACTS = 10_000

_REQUIRED_MEMBERS = frozenset(
    {
        "manifest.json",
        "nodes.ndjson",
        "seals.json",
        "artifacts-manifest.json",
        "COMPAT.json",
    }
)

# Keys whose values are redacted in redacted export mode (case-insensitive).
_SECRET_KEY_RE = re.compile(
    r"(password|passwd|secret|token|api[_-]?key|authorization|auth|credential|private[_-]?key)",
    re.IGNORECASE,
)


class TirError(Exception):
    """Base error for package operations."""


class TirVerificationError(TirError):
    """Raised when import finds a hash or structural mismatch."""


class TirLimitError(TirError):
    """Raised when a package exceeds safe size or count limits."""


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


def _safe_zip_name(name: str) -> None:
    """Reject path traversal and absolute paths inside the zip."""
    if not name or name.startswith("/") or name.startswith("\\"):
        raise TirVerificationError(f"unsafe zip member path: {name!r}")
    # Normalize separators and reject .. segments
    parts = name.replace("\\", "/").split("/")
    if ".." in parts or parts[0] == "":
        raise TirVerificationError(f"unsafe zip member path: {name!r}")
    if name.startswith("artifacts/") and not (
        name == "artifacts/" or name.startswith("artifacts/cas/")
    ):
        raise TirVerificationError(f"unexpected artifact path: {name!r}")


def _read_zip_member(zf: zipfile.ZipFile, name: str, *, budget: list[int]) -> bytes:
    """Read one member with per-entry and total uncompressed budgets.

    ``budget`` is a single-element list holding remaining total bytes.
    """
    _safe_zip_name(name)
    info = zf.getinfo(name)
    # Zip bombs often report huge file_size; reject before allocating.
    if info.file_size > MAX_UNCOMPRESSED_ENTRY_BYTES:
        raise TirLimitError(
            f"zip member {name!r} uncompressed size {info.file_size} "
            f"exceeds limit {MAX_UNCOMPRESSED_ENTRY_BYTES}"
        )
    if info.file_size > budget[0]:
        raise TirLimitError(
            f"zip total uncompressed size would exceed limit {MAX_TOTAL_UNCOMPRESSED_BYTES}"
        )
    data = zf.read(name)
    if len(data) > MAX_UNCOMPRESSED_ENTRY_BYTES:
        raise TirLimitError(f"zip member {name!r} expanded beyond per-entry limit")
    budget[0] -= len(data)
    return data


def _verify_node_record(rec: dict[str, Any]) -> None:
    """Recompute id from fields; raise if it does not match the stored id."""
    payload = rec["payload"]
    if not isinstance(payload, dict):
        raise TirVerificationError(f"node {rec.get('id')}: payload must be an object")
    try:
        ph = payload_hash(payload)
        expected = node_id(
            rec["tenant_id"],
            rec["trajectory_id"],
            rec.get("step_n"),
            int(rec["seq"]),
            rec["kind"],
            ph,
        )
    except (NodeValidationError, KeyError, TypeError, ValueError) as e:
        raise TirVerificationError(f"node {rec.get('id')}: {e}") from e
    if rec.get("id") != expected:
        raise TirVerificationError(
            f"node id mismatch for kind={rec.get('kind')} seq={rec.get('seq')}: "
            f"recorded={rec.get('id')} expected={expected}"
        )
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


def _redact_value(key: str, value: Any) -> Any:
    if _SECRET_KEY_RE.search(key):
        return "[REDACTED]"
    if isinstance(value, dict):
        return {k: _redact_value(str(k), v) for k, v in value.items()}
    if isinstance(value, list):
        return [_redact_value(key, item) for item in value]
    return value


def _redact_node(rec: dict[str, Any]) -> dict[str, Any]:
    """Return a copy of a node record with thoughts/secrets stripped for sharing."""
    kind = rec["kind"]
    payload = rec["payload"]
    if kind == "THOUGHT":
        new_payload: dict[str, Any] = {"redacted": True}
    elif isinstance(payload, dict):
        new_payload = {k: _redact_value(str(k), v) for k, v in payload.items()}
    else:
        new_payload = {"redacted": True}

    # Rebuild via Node so id matches redacted payload (export is a new package).
    node = Node(
        kind=kind,
        trajectory_id=rec["trajectory_id"],
        tenant_id=rec["tenant_id"],
        step_n=rec.get("step_n"),
        seq=int(rec["seq"]),
        payload=new_payload,
        ts=float(rec.get("ts") or 0.0),
    )
    return {
        "id": node.id,
        "trajectory_id": node.trajectory_id,
        "tenant_id": node.tenant_id,
        "step_n": node.step_n,
        "seq": node.seq,
        "kind": node.kind,
        "payload": node.payload,
        "ts": node.ts,
        "payload_hash": node.phash,
    }


def export_tir(
    node_log: NodeLog,
    trajectory_id: str,
    dest: str | Path,
    *,
    mode: PackageMode = "thin",
    artifacts: list[ArtifactRef] | None = None,
    artifact_bytes: dict[str, bytes] | None = None,
    tenant_id: str | None = None,
    redacted: bool = False,
) -> Path:
    """Export a trajectory from ``node_log`` to a ``.tir`` zip at ``dest``.

    Args:
        tenant_id: When set, only that tenant's nodes are exported (isolation).
        redacted: When True, strip thoughts and secret-like fields before export.
    """
    if mode not in ("thin", "fat"):
        raise TirError(f"unsupported mode {mode!r}; use thin or fat")

    nodes = node_log.list_nodes(trajectory_id, tenant_id=tenant_id)
    if not nodes:
        raise TirError(f"no nodes for trajectory_id={trajectory_id!r}")

    export_tenant = nodes[0]["tenant_id"]
    for n in nodes:
        if n["tenant_id"] != export_tenant:
            raise TirError(
                "trajectory contains mixed tenant_id values; pass tenant_id= to scope export"
            )
        _verify_node_record(n)

    if redacted:
        nodes = [_redact_node(n) for n in nodes]
        # Fat artifacts may hold secrets; omit bytes in redacted mode.
        if mode == "fat":
            mode = "thin"
            artifacts = []
            artifact_bytes = {}

    artifacts = list(artifacts or [])
    artifact_bytes = dict(artifact_bytes or {})
    if mode == "fat":
        for ref in artifacts:
            data = artifact_bytes.get(ref.content_hash)
            if data is None:
                raise TirError(f"fat export missing bytes for content_hash={ref.content_hash}")
            if _content_hash(data) != ref.content_hash:
                raise TirError(f"artifact bytes do not match content_hash={ref.content_hash}")
            if len(data) > MAX_UNCOMPRESSED_ENTRY_BYTES:
                raise TirLimitError(f"artifact {ref.content_hash} exceeds size limit")
        if len(artifacts) > MAX_ARTIFACTS:
            raise TirLimitError(f"too many artifacts: {len(artifacts)} > {MAX_ARTIFACTS}")
    else:
        artifact_bytes = {}

    seals = _seals_from_nodes(nodes)
    manifest = {
        "package_format": PACKAGE_FORMAT_VERSION,
        "trajectory_id": trajectory_id,
        "tenant_id": export_tenant,
        "mode": mode,
        "node_count": len(nodes),
        "seal_count": len(seals),
        "signature": None,
        "redacted": redacted,
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
                shard = h[:2]
                zf.writestr(f"artifacts/cas/{shard}/{h}", data)

    return dest_path


def load_tir(path: str | Path, *, verify: bool = True) -> TirPackage:
    """Load and verify a ``.tir`` zip without writing to a NodeLog.

    Verification is required for untrusted packages. To intentionally skip
    verification (tests / recovery tools only), call
    :func:`load_tir_unverified`.
    """
    if not verify:
        raise TirError(
            "load_tir(verify=False) is disabled for safety; "
            "use load_tir_unverified() if you explicitly accept an unverified package"
        )
    return _load_tir_impl(path, verify=True)


def load_tir_unverified(path: str | Path) -> TirPackage:
    """Load a package without hash verification. UNSAFE for untrusted input."""
    return _load_tir_impl(path, verify=False)


def _load_tir_impl(path: str | Path, *, verify: bool) -> TirPackage:
    path = Path(path)
    if not path.is_file():
        raise TirError(f"package not found: {path}")

    with zipfile.ZipFile(path, "r") as zf:
        names = zf.namelist()
        if len(names) > MAX_ZIP_ENTRIES:
            raise TirLimitError(f"too many zip entries: {len(names)} > {MAX_ZIP_ENTRIES}")
        name_set = set(names)
        missing = _REQUIRED_MEMBERS - name_set
        if missing:
            raise TirError(f"package missing files: {sorted(missing)}")

        for name in names:
            _safe_zip_name(name)

        budget = [MAX_TOTAL_UNCOMPRESSED_BYTES]
        manifest = json.loads(_read_zip_member(zf, "manifest.json", budget=budget))
        seals = json.loads(_read_zip_member(zf, "seals.json", budget=budget))
        artifacts_manifest = json.loads(
            _read_zip_member(zf, "artifacts-manifest.json", budget=budget)
        )
        # COMPAT is advisory for now but must still be readable under budget.
        _read_zip_member(zf, "COMPAT.json", budget=budget)

        nodes_raw = _read_zip_member(zf, "nodes.ndjson", budget=budget).decode("utf-8")
        nodes: list[dict[str, Any]] = []
        for line in nodes_raw.splitlines():
            line = line.strip()
            if not line:
                continue
            nodes.append(json.loads(line))
            if len(nodes) > MAX_NODES:
                raise TirLimitError(f"too many nodes: > {MAX_NODES}")

        if isinstance(artifacts_manifest, list) and len(artifacts_manifest) > MAX_ARTIFACTS:
            raise TirLimitError(f"too many artifact manifest entries: > {MAX_ARTIFACTS}")

        artifact_bytes: dict[str, bytes] = {}
        for name in names:
            if name.startswith("artifacts/cas/") and not name.endswith("/"):
                data = _read_zip_member(zf, name, budget=budget)
                h = Path(name).name
                if len(h) != 64 or any(c not in "0123456789abcdef" for c in h.lower()):
                    raise TirVerificationError(f"invalid artifact content hash name: {h!r}")
                if _content_hash(data) != h:
                    raise TirVerificationError(
                        f"embedded artifact name {h} does not match content hash"
                    )
                artifact_bytes[h] = data
                if len(artifact_bytes) > MAX_ARTIFACTS:
                    raise TirLimitError(f"too many embedded artifacts: > {MAX_ARTIFACTS}")

    if verify:
        if not nodes:
            raise TirVerificationError("package has no nodes")
        for n in nodes:
            _verify_node_record(n)
        expected_seals = _seals_from_nodes(nodes)
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

    Does not acquire a writer lease (README §9). Verification cannot be disabled
    on import — forged packages must not enter a durable log.
    """
    if not verify:
        raise TirError(
            "import_tir refuses verify=False; forged packages must not enter a NodeLog. "
            "Use load_tir_unverified() only for inspection, never for durable import."
        )
    pkg = load_tir(path, verify=True)
    for n in pkg.nodes:
        node = Node(
            kind=n["kind"],
            trajectory_id=n["trajectory_id"],
            tenant_id=n["tenant_id"],
            step_n=n.get("step_n"),
            seq=int(n["seq"]),
            payload=n["payload"],
            ts=float(n.get("ts") or 0.0),
        )
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
