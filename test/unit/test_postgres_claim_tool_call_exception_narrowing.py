"""Regression test for issue #96: claim_tool_call must not swallow non-conflict errors.

Uses minimal purpose-built fakes (not test_postgres_node_log.py's _FakeStore)
so this file stays self-contained and independently mergeable.
"""

from __future__ import annotations

import pytest
from psycopg.errors import UniqueViolation

from drivers.postgres.log import PostgresNodeLog


class _RaisingCursor:
    def __init__(self, exc_to_raise: Exception) -> None:
        self._exc_to_raise = exc_to_raise
        self._select_done = False

    def __enter__(self) -> _RaisingCursor:
        return self

    def __exit__(self, exc_type, exc, tb) -> None:
        return None

    def execute(self, sql: str, params=None) -> None:
        normalized = " ".join(sql.lower().split())
        if (
            normalized.startswith("create table")
            or normalized.startswith("create unique index")
            or normalized.startswith("create index")
        ):
            # PostgresNodeLog._ensure_schema DDL; no-op for this fake.
            return
        if normalized.startswith("select 1 from nodes"):
            # No existing claimant: fall through to the INSERT attempt.
            self._select_done = True
            return
        if normalized.startswith("insert into nodes"):
            raise self._exc_to_raise
        raise AssertionError(f"unexpected SQL in fake cursor: {sql!r}")

    def fetchone(self):
        return None


class _RaisingConnection:
    """Fake psycopg connection whose INSERT step raises a chosen exception."""

    def __init__(self, exc_to_raise: Exception) -> None:
        self._exc_to_raise = exc_to_raise
        self.committed = 0
        self.rolled_back = 0

    def cursor(self):
        return _RaisingCursor(self._exc_to_raise)

    def commit(self) -> None:
        self.committed += 1

    def rollback(self) -> None:
        self.rolled_back += 1

    def close(self) -> None:
        return None


def _make_log(exc_to_raise: Exception) -> tuple[PostgresNodeLog, _RaisingConnection]:
    conn = _RaisingConnection(exc_to_raise)
    log = PostgresNodeLog(conn)
    # _ensure_schema's DDL calls happened against the same fake cursor above,
    # which only special-cases SELECT/INSERT INTO nodes; reset counters so
    # assertions below only reflect claim_tool_call's own commit/rollback.
    conn.committed = 0
    conn.rolled_back = 0
    return log, conn


def test_claim_tool_call_returns_false_on_unique_violation():
    """A genuine lost race (UniqueViolation) still returns False, not an exception."""
    log, conn = _make_log(UniqueViolation("duplicate key"))

    claimed = log.claim_tool_call(
        step_n=1,
        payload={"tool": "x"},
        trajectory_id="t1",
        tenant_id="demo",
        seq=1,
    )

    assert claimed is False
    assert conn.rolled_back == 1
    assert conn.committed == 0


def test_claim_tool_call_propagates_unrelated_exception():
    """A dropped connection or other real error must not be silently swallowed as False."""
    log, conn = _make_log(RuntimeError("connection dropped"))

    with pytest.raises(RuntimeError, match="connection dropped"):
        log.claim_tool_call(
            step_n=1,
            payload={"tool": "x"},
            trajectory_id="t1",
            tenant_id="demo",
            seq=1,
        )

    assert conn.rolled_back == 1
    assert conn.committed == 0
