import contextlib
import json
import sqlite3
import threading
from typing import Any

from trajectory_ir.runtime.nodes import Node


class SlotConflictError(ValueError):
    """Raised when a different payload is written to an occupied (trajectory, step,
    seq, kind) slot."""


class NodeLog:
    """Append-only, content-addressed node log backed by SQLite.

    Appends are idempotent: replaying an append for a node whose id already
    exists is a no-op (`INSERT OR IGNORE`). This is what makes DBOS's
    workflow replay safe to layer on top of -- re-running an already-appended
    step produces the same node id and is silently absorbed instead of
    duplicating history, so there is no separate "seal" operation needed.

    Concurrent writers are serialized with a lock and ``BEGIN IMMEDIATE`` so
    the block-and-gate claim path cannot double-run a non-idempotent tool.
    """

    def __init__(self, db_path: str):
        # check_same_thread=False: the durable backend replays a crashed
        # workflow on its own recovery thread, so the log must be writable from a
        # thread other than the one that opened it. Access is serialized via
        # self._lock; only one thread mutates the connection at a time.
        self._lock = threading.RLock()
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
        # Logical slot uniqueness per tenant: same content id is still
        # INSERT OR IGNORE; a different payload at the same slot is rejected.
        self._conn.execute(
            """
            CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_slot
            ON nodes (tenant_id, trajectory_id, IFNULL(step_n, -1), seq, kind)
            """,
        )
        self._conn.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_nodes_traj_tenant
            ON nodes (trajectory_id, tenant_id)
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
        with self._lock:
            self._conn.execute(
                "INSERT OR IGNORE INTO nodes "
                "(id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts) "
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
            # INSERT OR IGNORE also swallows unique-slot conflicts. Detect a
            # different payload occupying this logical slot and fail closed.
            cur = self._conn.execute(
                "SELECT id FROM nodes WHERE tenant_id = ? AND trajectory_id = ? "
                "AND IFNULL(step_n, -1) = IFNULL(?, -1) AND seq = ? AND kind = ?",
                (tenant_id, trajectory_id, step_n, seq, kind),
            )
            row = cur.fetchone()
            if row is not None and row[0] != node.id:
                self._conn.rollback()
                raise SlotConflictError(
                    f"slot conflict trajectory={trajectory_id!r} step={step_n} "
                    f"seq={seq} kind={kind!r}: different payload already stored"
                )
            self._conn.commit()
        return node

    def claim_tool_call(
        self,
        step_n: int,
        payload: dict,
        trajectory_id: str,
        tenant_id: str,
        seq: int,
    ) -> bool:
        """Atomically claim a TOOL_CALL slot.

        Returns True if this caller inserted the TOOL_CALL (may run the tool).
        Returns False if a TOOL_CALL already occupied this (trajectory, step, seq).
        """
        node = Node(
            kind="TOOL_CALL",
            trajectory_id=trajectory_id,
            tenant_id=tenant_id,
            step_n=step_n,
            seq=seq,
            payload=payload,
        )
        with self._lock:
            try:
                self._conn.execute("BEGIN IMMEDIATE")
                cur = self._conn.execute(
                    "SELECT 1 FROM nodes WHERE tenant_id = ? AND trajectory_id = ? "
                    "AND step_n = ? AND kind = ? AND seq = ? LIMIT 1",
                    (tenant_id, trajectory_id, step_n, "TOOL_CALL", seq),
                )
                if cur.fetchone() is not None:
                    self._conn.execute("COMMIT")
                    return False
                self._conn.execute(
                    "INSERT INTO nodes "
                    "(id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts) "
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
                self._conn.execute("COMMIT")
                return True
            except sqlite3.IntegrityError:
                self._conn.execute("ROLLBACK")
                return False
            except Exception:
                with contextlib.suppress(sqlite3.Error):
                    self._conn.execute("ROLLBACK")
                raise

    def has(
        self,
        trajectory_id: str,
        tenant_id: str,
        step_n: int,
        kind: str,
        seq: int | None = None,
    ) -> bool:
        """Does a node of `kind` exist for this trajectory/step scoped to `tenant_id`?

        `tenant_id` is required to enforce multi-tenant isolation. Callers that
        genuinely need cross-tenant existence checks (e.g., administrative or
        diagnostic tools) must use `has_all_tenants` explicitly instead.

        `seq` narrows the question to one exact node slot. Without it, a step
        containing several tool calls cannot distinguish them: one completed
        call's TOOL_RESULT would answer for a different, interrupted call. Left
        as None the query is unscoped, which is the right question for
        step-level kinds (DECISION, COMMIT_STEP) that occur at most once.
        """
        if not tenant_id:
            raise ValueError("tenant_id must be a non-empty string")
        return self._has(trajectory_id, tenant_id=tenant_id, step_n=step_n, kind=kind, seq=seq)

    def has_all_tenants(
        self,
        trajectory_id: str,
        step_n: int,
        kind: str,
        seq: int | None = None,
    ) -> bool:
        """Does a node of `kind` exist for this trajectory/step across any tenant?

        This bypasses tenant isolation and should only be used for administrative
        diagnostics or debugging tools, not standard execution paths.
        """
        return self._has(trajectory_id, tenant_id=None, step_n=step_n, kind=kind, seq=seq)

    def _has(
        self,
        trajectory_id: str,
        *,
        tenant_id: str | None,
        step_n: int,
        kind: str,
        seq: int | None,
    ) -> bool:
        sql = "SELECT 1 FROM nodes WHERE trajectory_id = ? AND step_n = ? AND kind = ?"
        params: list[object] = [trajectory_id, step_n, kind]
        if tenant_id is not None:
            sql += " AND tenant_id = ?"
            params.append(tenant_id)
        if seq is not None:
            sql += " AND seq = ?"
            params.append(seq)
        with self._lock:
            cur = self._conn.execute(sql + " LIMIT 1", params)
            return cur.fetchone() is not None

    def count(self, node_id: str) -> int:
        with self._lock:
            cur = self._conn.execute("SELECT COUNT(*) FROM nodes WHERE id = ?", (node_id,))
            return int(cur.fetchone()[0])

    def list_nodes(
        self,
        trajectory_id: str,
        *,
        tenant_id: str,
    ) -> list[dict[str, Any]]:
        """Return stored nodes for a trajectory ordered by step then seq.

        ``tenant_id`` is required so this method always enforces tenant
        isolation at the read path. Callers that genuinely need rows across
        tenants (e.g. ``export_tir``'s own mixed-tenant check) must use
        ``list_nodes_all_tenants`` explicitly instead.
        """
        return self._list_nodes(trajectory_id, tenant_id=tenant_id)

    def list_nodes_all_tenants(self, trajectory_id: str) -> list[dict[str, Any]]:
        """Return stored nodes for a trajectory across all tenants.

        This bypasses tenant isolation and should only be used by code that
        performs its own tenant-scoping check on the result, such as
        ``export_tir`` detecting a trajectory with mixed ``tenant_id`` values.
        """
        return self._list_nodes(trajectory_id, tenant_id=None)

    def _list_nodes(
        self,
        trajectory_id: str,
        *,
        tenant_id: str | None,
    ) -> list[dict[str, Any]]:
        sql = """
            SELECT id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts
            FROM nodes
            WHERE trajectory_id = ?
            """
        params: list[object] = [trajectory_id]
        if tenant_id is not None:
            sql += " AND tenant_id = ?"
            params.append(tenant_id)
        sql += " ORDER BY COALESCE(step_n, -1), seq"
        with self._lock:
            cur = self._conn.execute(sql, params)
            rows: list[dict[str, Any]] = []
            for row in cur.fetchall():
                rows.append(
                    {
                        "id": row[0],
                        "trajectory_id": row[1],
                        "tenant_id": row[2],
                        "step_n": row[3],
                        "seq": row[4],
                        "kind": row[5],
                        "payload": json.loads(row[6]),
                        "ts": row[7],
                    }
                )
            return rows

    def close(self):
        with self._lock:
            self._conn.close()

    def __del__(self):
        # Client SDK call sites construct a fresh NodeLog per function call
        # and never close it explicitly. Without this, connections only get
        # released whenever the garbage collector happens to get to them,
        # which over a long running agent session means an unbounded number
        # of open SQLite connections. This is a best-effort backstop, not a
        # substitute for callers using close()/a context manager when they
        # can.
        with contextlib.suppress(Exception):
            self._conn.close()
