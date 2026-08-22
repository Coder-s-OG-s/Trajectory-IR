"""PostgreSQL backed NodeLog matching SQLite semantics.

Hash identity stays client side (RFC 8785 JCS + SHA-256 via
:class:`~trajectory_ir.runtime.nodes.Node`). This driver only persists rows.

Schema (infrastructure.md section 1.2, aligned with SQLite NodeLog)::

    nodes(
      id TEXT PRIMARY KEY,
      trajectory_id TEXT NOT NULL,
      tenant_id TEXT NOT NULL,
      step_n INTEGER,
      seq INTEGER NOT NULL,
      kind TEXT NOT NULL,
      payload_json TEXT NOT NULL,
      ts DOUBLE PRECISION NOT NULL
    )

Logical slot uniqueness: (tenant_id, trajectory_id, coalesce(step_n, -1), seq, kind).
"""

from __future__ import annotations

import json
import os
import threading
from typing import Any

from trajectory_ir.runtime.log import SlotConflictError
from trajectory_ir.runtime.nodes import Node


def _require_psycopg():
    try:
        import psycopg
        from psycopg.rows import tuple_row
    except ImportError as exc:
        raise ImportError(
            "psycopg is required for PostgresNodeLog; "
            'pip install "psycopg[binary]>=3" or pip install -e ".[postgres]"'
        ) from exc
    return psycopg, tuple_row


class PostgresNodeLog:
    """Append only, content addressed node log backed by PostgreSQL.

    Public methods mirror :class:`~trajectory_ir.runtime.log.NodeLog` so
    callers can swap storage without changing seal or gate code.
    """

    def __init__(self, conn: Any) -> None:
        """Wrap an open psycopg connection (autocommit disabled)."""
        self._lock = threading.RLock()
        self._conn = conn
        self._ensure_schema()

    def _ensure_schema(self) -> None:
        with self._lock:
            with self._conn.cursor() as cur:
                cur.execute(
                    """
                    CREATE TABLE IF NOT EXISTS nodes (
                        id TEXT PRIMARY KEY,
                        trajectory_id TEXT NOT NULL,
                        tenant_id TEXT NOT NULL,
                        step_n INTEGER,
                        seq INTEGER NOT NULL,
                        kind TEXT NOT NULL,
                        payload_json TEXT NOT NULL,
                        ts DOUBLE PRECISION NOT NULL
                    )
                    """
                )
                cur.execute(
                    """
                    CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_slot
                    ON nodes (
                        tenant_id,
                        trajectory_id,
                        (COALESCE(step_n, -1)),
                        seq,
                        kind
                    )
                    """
                )
                cur.execute(
                    """
                    CREATE INDEX IF NOT EXISTS idx_nodes_traj_tenant
                    ON nodes (trajectory_id, tenant_id)
                    """
                )
            self._conn.commit()

    def _slot_owner_id(
        self,
        cur: Any,
        *,
        tenant_id: str,
        trajectory_id: str,
        step_n: int | None,
        seq: int,
        kind: str,
    ) -> str | None:
        cur.execute(
            """
            SELECT id FROM nodes
            WHERE tenant_id = %s AND trajectory_id = %s
              AND COALESCE(step_n, -1) = COALESCE(%s, -1)
              AND seq = %s AND kind = %s
            """,
            (tenant_id, trajectory_id, step_n, seq, kind),
        )
        row = cur.fetchone()
        return row[0] if row is not None else None

    def append(
        self,
        kind: str,
        step_n: int | None,
        payload: dict,
        trajectory_id: str,
        tenant_id: str,
        seq: int,
    ) -> Node:
        """Append a node; match SQLite NodeLog slot conflict semantics.

        Postgres only ignores primary key conflicts with ``ON CONFLICT (id)``.
        A different payload at the same logical slot hits ``idx_nodes_slot`` and
        must become :class:`SlotConflictError` (SQLite ``INSERT OR IGNORE`` +
        post-check does the same).
        """
        # Import here so the module loads without psycopg installed.
        from psycopg.errors import UniqueViolation

        node = Node(
            kind=kind,
            trajectory_id=trajectory_id,
            tenant_id=tenant_id,
            step_n=step_n,
            seq=seq,
            payload=payload,
        )
        with self._lock:
            try:
                with self._conn.cursor() as cur:
                    cur.execute(
                        """
                        INSERT INTO nodes
                        (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts)
                        VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
                        ON CONFLICT (id) DO NOTHING
                        """,
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
                    owner = self._slot_owner_id(
                        cur,
                        tenant_id=tenant_id,
                        trajectory_id=trajectory_id,
                        step_n=step_n,
                        seq=seq,
                        kind=kind,
                    )
                    if owner is not None and owner != node.id:
                        self._conn.rollback()
                        raise SlotConflictError(
                            f"slot conflict trajectory={trajectory_id!r} step={step_n} "
                            f"seq={seq} kind={kind!r}: different payload already stored"
                        )
                self._conn.commit()
            except UniqueViolation:
                # Slot unique index fired (different content id, same slot).
                self._conn.rollback()
                with self._conn.cursor() as cur:
                    owner = self._slot_owner_id(
                        cur,
                        tenant_id=tenant_id,
                        trajectory_id=trajectory_id,
                        step_n=step_n,
                        seq=seq,
                        kind=kind,
                    )
                if owner is not None and owner != node.id:
                    raise SlotConflictError(
                        f"slot conflict trajectory={trajectory_id!r} step={step_n} "
                        f"seq={seq} kind={kind!r}: different payload already stored"
                    ) from None
                # Same id concurrent insert or unexpected unique path: treat as ok.
        return node

    def claim_tool_call(
        self,
        step_n: int,
        payload: dict,
        trajectory_id: str,
        tenant_id: str,
        seq: int,
    ) -> bool:
        """Atomically claim a TOOL_CALL slot (single winner under concurrency)."""
        # Import here so the module loads without psycopg installed.
        from psycopg.errors import UniqueViolation

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
                with self._conn.cursor() as cur:
                    cur.execute(
                        """
                        SELECT 1 FROM nodes
                        WHERE tenant_id = %s AND trajectory_id = %s
                          AND step_n = %s AND kind = %s AND seq = %s
                        LIMIT 1
                        FOR UPDATE
                        """,
                        (tenant_id, trajectory_id, step_n, "TOOL_CALL", seq),
                    )
                    if cur.fetchone() is not None:
                        self._conn.commit()
                        return False
                    cur.execute(
                        """
                        INSERT INTO nodes
                        (id, trajectory_id, tenant_id, step_n, seq, kind, payload_json, ts)
                        VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
                        """,
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
                return True
            except UniqueViolation:
                self._conn.rollback()
                # Another writer won the slot between our SELECT and INSERT.
                return False
            except Exception:
                self._conn.rollback()
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

        Matches SQLite NodeLog semantics. Callers needing cross-tenant existence
        checks must use `has_all_tenants` explicitly.
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

        Bypasses tenant isolation for administrative diagnostics and debugging.
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
        sql = "SELECT 1 FROM nodes WHERE trajectory_id = %s AND step_n = %s AND kind = %s"
        params: list[object] = [trajectory_id, step_n, kind]
        if tenant_id is not None:
            sql += " AND tenant_id = %s"
            params.append(tenant_id)
        if seq is not None:
            sql += " AND seq = %s"
            params.append(seq)
        sql += " LIMIT 1"
        with self._lock, self._conn.cursor() as cur:
            cur.execute(sql, params)
            return cur.fetchone() is not None

    def count(self, node_id: str) -> int:
        with self._lock, self._conn.cursor() as cur:
            cur.execute("SELECT COUNT(*) FROM nodes WHERE id = %s", (node_id,))
            row = cur.fetchone()
            return int(row[0]) if row else 0

    def list_nodes(
        self,
        trajectory_id: str,
        *,
        tenant_id: str,
    ) -> list[dict[str, Any]]:
        return self._list_nodes(trajectory_id, tenant_id=tenant_id)

    def list_nodes_all_tenants(self, trajectory_id: str) -> list[dict[str, Any]]:
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
            WHERE trajectory_id = %s
            """
        params: list[object] = [trajectory_id]
        if tenant_id is not None:
            sql += " AND tenant_id = %s"
            params.append(tenant_id)
        sql += " ORDER BY COALESCE(step_n, -1), seq"
        with self._lock, self._conn.cursor() as cur:
            cur.execute(sql, params)
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

    def close(self) -> None:
        with self._lock:
            self._conn.close()


def open_postgres_node_log(dsn: str | None = None) -> PostgresNodeLog:
    """Open a :class:`PostgresNodeLog` from a DSN or environment.

    Resolution order:
    1. Explicit ``dsn`` argument
    2. ``TRAJIR_DATABASE_URL``
    3. ``DATABASE_URL``
    """
    psycopg, _tuple_row = _require_psycopg()
    resolved = dsn or os.environ.get("TRAJIR_DATABASE_URL") or os.environ.get("DATABASE_URL")
    if not resolved:
        raise ValueError(
            "PostgreSQL DSN required: pass dsn= or set TRAJIR_DATABASE_URL / DATABASE_URL"
        )
    conn = psycopg.connect(resolved)
    return PostgresNodeLog(conn)
