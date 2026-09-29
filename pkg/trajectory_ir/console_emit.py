"""Optional console sink for hosts. Stdlib only.

TRAJIR_CONSOLE_SINK=file writes NDJSON under TRAJIR_CONSOLE_DATA/trajectories.
TRAJIR_CONSOLE_SINK=http POSTs to TRAJIR_CONSOLE_URL/v1/events.
Unset means no sink. Failures are logged and never raised to the caller.
"""

from __future__ import annotations

import json
import logging
import os
import urllib.error
import urllib.request
import uuid
from datetime import UTC, datetime
from typing import Any

log = logging.getLogger("trajectory_ir.console_emit")

SCHEMA = "console-events-v1"


def from_env():
    kind = os.environ.get("TRAJIR_CONSOLE_SINK", "").strip().lower()
    if kind == "file":
        return FileSink(os.environ.get("TRAJIR_CONSOLE_DATA", ""))
    if kind == "http":
        return HTTPSink(
            os.environ.get("TRAJIR_CONSOLE_URL", "http://127.0.0.1:8787"),
            os.environ.get("TRAJIR_CONSOLE_TOKEN", ""),
        )
    return None


class FileSink:
    def __init__(self, root: str):
        self.root = root

    def emit(self, event: dict) -> None:
        if not self.root:
            raise ValueError("file sink data dir required")
        tid = str(event.get("trajectory_id") or "")
        if not tid or "/" in tid or "\\" in tid or ".." in tid or len(tid) > 200:
            raise ValueError("bad trajectory_id")
        folder = os.path.join(self.root, "trajectories")
        os.makedirs(folder, exist_ok=True)
        path = os.path.join(folder, tid + ".ndjson")
        line = json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n"
        with open(path, "a", encoding="utf-8") as fh:
            fh.write(line)


class HTTPSink:
    def __init__(self, url: str, token: str = ""):
        self.url = url.rstrip("/") + "/v1/events"
        self.token = token

    def emit(self, event: dict) -> None:
        data = json.dumps(event, sort_keys=True).encode("utf-8")
        req = urllib.request.Request(self.url, data=data, method="POST")
        req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", "Bearer " + self.token)
        try:
            with urllib.request.urlopen(req, timeout=2) as resp:
                if resp.status != 201:
                    raise RuntimeError(f"console http sink: status {resp.status}")
        except urllib.error.HTTPError as exc:
            raise RuntimeError(f"console http sink: status {exc.code}") from exc


def emit(sink: Any, event: dict) -> None:
    if sink is None:
        return
    try:
        ev = dict(event)
        ev.setdefault("schema_version", SCHEMA)
        ev.setdefault("id", uuid.uuid4().hex)
        ev.setdefault("ts", datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ"))
        ev.setdefault("source", "python")
        ev.setdefault("payload", {})
        sink.emit(ev)
    except Exception:
        log.warning("console sink failed", exc_info=True)


def note_node(
    sink: Any,
    *,
    trajectory_id: str,
    tenant_id: str,
    node: Any,
    source: str = "python",
    runtime: str = "python",
) -> None:
    payload = {
        "node_id": node.id,
        "kind": node.kind,
        "seq": node.seq,
        "content_hash": node.phash,
    }
    if node.step_n is not None:
        payload["step_n"] = node.step_n
    emit(sink, {
        "kind": "node.appended",
        "source": source,
        "runtime": runtime,
        "trajectory_id": trajectory_id,
        "tenant_id": tenant_id,
        "payload": payload,
    })


def note_seal(
    sink: Any,
    *,
    trajectory_id: str,
    tenant_id: str,
    node: Any,
    step_n: int,
    plan: dict | None,
    source: str = "python",
    runtime: str = "python",
) -> None:
    payload: dict[str, Any] = {
        "node_id": node.id,
        "step_n": step_n,
        "content_hash": node.phash,
    }
    names = []
    for call in (plan or {}).get("tool_calls") or []:
        if isinstance(call, dict) and call.get("name"):
            names.append(call["name"])
    if names:
        payload["tool_names"] = names
    emit(sink, {
        "kind": "seal.created",
        "source": source,
        "runtime": runtime,
        "trajectory_id": trajectory_id,
        "tenant_id": tenant_id,
        "payload": payload,
    })


def note_package(
    sink: Any,
    *,
    kind: str,
    trajectory_id: str,
    tenant_id: str,
    path: str,
    mode: str,
    redacted: bool,
    nbytes: int,
    member_count: int,
    node_count: int,
    ok: bool,
    error: str = "",
    source: str = "python",
    runtime: str = "python",
) -> None:
    payload: dict[str, Any] = {
        "path": path,
        "mode": mode,
        "redacted": bool(redacted),
        "bytes": int(nbytes),
        "member_count": int(member_count),
        "node_count": int(node_count),
    }
    if kind == "import.completed":
        payload["verify_ok"] = bool(ok)
    else:
        payload["ok"] = bool(ok)
    if error:
        payload["error"] = error
    emit(sink, {
        "kind": kind,
        "source": source,
        "runtime": runtime,
        "trajectory_id": trajectory_id,
        "tenant_id": tenant_id,
        "payload": payload,
    })
