import json
import sqlite3

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
        # check_same_thread=False: the durable backend replays a crashed
        # workflow on its own recovery thread, so the log must be writable from a
        # thread other than the one that opened it. Only one thread executes a
        # given workflow body at a time, so no extra locking is needed.
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
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

    def append(
        self,
        kind: str,
        step_n: int | None,
        payload: dict,
        trajectory_id: str,
        tenant_id: str,
        seq: int,
    ) -> Node:
        node = Node(
            kind=kind,
            trajectory_id=trajectory_id,
            tenant_id=tenant_id,
            step_n=step_n,
            seq=seq,
            payload=payload,
        )
        self._conn.execute(
            "INSERT OR IGNORE INTO nodes (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (
                node.id,
                node.trajectory_id,
                node.tenant_id,
                node.step_n,
                node.seq,
                node.kind,
                json.dumps(node.payload),
                node.ts,
            ),
        )
        self._conn.commit()
        return node

    def has(self, trajectory_id: str, step_n: int, kind: str, seq: int | None = None) -> bool:
        """Does a node of `kind` exist for this trajectory/step?

        `seq` narrows the question to one exact node slot. Without it, a step
        containing several tool calls cannot distinguish them: one completed
        call's TOOL_RESULT would answer for a different, interrupted call. Left
        as None the query is unscoped, which is the right question for
        step-level kinds (DECISION, COMMIT_STEP) that occur at most once.
        """
        sql = "SELECT 1 FROM nodes WHERE trajectory_id = ? AND step_n = ? AND kind = ?"
        params: list[object] = [trajectory_id, step_n, kind]
        if seq is not None:
            sql += " AND seq = ?"
            params.append(seq)
        cur = self._conn.execute(sql + " LIMIT 1", params)
        return cur.fetchone() is not None

    def count(self, node_id: str) -> int:
        cur = self._conn.execute("SELECT COUNT(*) FROM nodes WHERE id = ?", (node_id,))
        return cur.fetchone()[0]

    def close(self):
        self._conn.close()
