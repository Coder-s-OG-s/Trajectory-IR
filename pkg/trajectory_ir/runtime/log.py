import json
import sqlite3
from typing import Optional

from trajectory_ir.runtime.nodes import Node


class NodeLog:
    """Append-only, content-addressed node log backed by SQLite.

    Appends are idempotent: replaying an append for a node whose id already
    exists is a no-op (`INSERT OR IGNORE`). This is what makes DBOS's
    workflow replay safe to layer on top of -- re-running an already-appended
    step produces the same node id and is silently absorbed instead of
    duplicating history, so there is no separate "seal" operation needed.
    """

    def __init__(self, db_path: str):
        self._conn = sqlite3.connect(db_path)
        self._conn.execute(
            """
            CREATE TABLE IF NOT EXISTS nodes (
                id TEXT PRIMARY KEY,
                trajectory_id TEXT NOT NULL,
                tenant_id TEXT NOT NULL,
                step_n INTEGER,
                seq INTEGER NOT NULL,
                kind TEXT NOT NULL,
                payload_json TEXT NOT NULL,
                ts REAL NOT NULL
            )
            """,
        )
        self._conn.commit()

    def append(self, kind: str, step_n: Optional[int], payload: dict, trajectory_id: str, tenant_id: str, seq: int) -> Node:
        node = Node(
            kind=kind, trajectory_id=trajectory_id, tenant_id=tenant_id,
            step_n=step_n, seq=seq, payload=payload,
        )
        self._conn.execute(
            "INSERT OR IGNORE INTO nodes (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (node.id, node.trajectory_id, node.tenant_id, node.step_n, node.seq, node.kind, json.dumps(node.payload), node.ts),
        )
        self._conn.commit()
        return node

    def has(self, trajectory_id: str, step_n: int, kind: str) -> bool:
        cur = self._conn.execute(
            "SELECT 1 FROM nodes WHERE trajectory_id = ? AND step_n = ? AND kind = ? LIMIT 1",
            (trajectory_id, step_n, kind),
        )
        return cur.fetchone() is not None

    def count(self, node_id: str) -> int:
        cur = self._conn.execute("SELECT COUNT(*) FROM nodes WHERE id = ?", (node_id,))
        return cur.fetchone()[0]

    def close(self):
        self._conn.close()
