"""Tests for PostgresNodeLog (in memory fake connection; no live server required)."""

from __future__ import annotations

import json
import os
import threading
from typing import Any

import pytest

from drivers.postgres.log import PostgresNodeLog, open_postgres_node_log
from trajectory_ir.runtime.log import SlotConflictError


class _FakeCursor:
    def __init__(self, store: _FakeStore) -> None:
        self._store = store
        self._result: list[tuple] = []

    def __enter__(self) -> _FakeCursor:
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def execute(self, sql: str, params: tuple | list | None = None) -> None:
        self._result = self._store.execute(sql, params or ())

    def fetchone(self) -> tuple | None:
        return self._result[0] if self._result else None

    def fetchall(self) -> list[tuple]:
        return list(self._result)


class _FakeStore:
    """Minimal SQL surface for the PostgresNodeLog methods under test."""

    def __init__(self) -> None:
        self.rows: dict[str, dict[str, Any]] = {}
        self.committed = True

    def execute(self, sql: str, params: tuple | list) -> list[tuple]:
        s = " ".join(sql.split()).lower()
        if s.startswith(("create table", "create unique index", "create index")):
            return []
        if s.startswith("insert into nodes") and "on conflict" in s:
            (
                node_id,
                trajectory_id,
                tenant_id,
                step_n,
                seq,
                kind,
                payload_json,
                ts,
            ) = params
            if node_id not in self.rows:
                self.rows[node_id] = {
                    "id": node_id,
                    "trajectory_id": trajectory_id,
                    "tenant_id": tenant_id,
                    "step_n": step_n,
                    "seq": seq,
                    "kind": kind,
                    "payload_json": payload_json,
                    "ts": ts,
                }
            return []
        if s.startswith("insert into nodes") and "on conflict" not in s:
            (
                node_id,
                trajectory_id,
                tenant_id,
                step_n,
                seq,
                kind,
                payload_json,
                ts,
            ) = params
            # Slot unique check
            for row in self.rows.values():
                if (
                    row["tenant_id"] == tenant_id
                    and row["trajectory_id"] == trajectory_id
                    and row["step_n"] == step_n
                    and row["seq"] == seq
                    and row["kind"] == kind
                ):
                    raise RuntimeError("unique_violation")
            self.rows[node_id] = {
                "id": node_id,
                "trajectory_id": trajectory_id,
                "tenant_id": tenant_id,
                "step_n": step_n,
                "seq": seq,
                "kind": kind,
                "payload_json": payload_json,
                "ts": ts,
            }
            return []
        if "select id from nodes" in s and "coalesce" in s:
            tenant_id, trajectory_id, step_n, seq, kind = params
            for row in self.rows.values():
                sn = row["step_n"] if row["step_n"] is not None else -1
                pn = step_n if step_n is not None else -1
                if (
                    row["tenant_id"] == tenant_id
                    and row["trajectory_id"] == trajectory_id
                    and sn == pn
                    and row["seq"] == seq
                    and row["kind"] == kind
                ):
                    return [(row["id"],)]
            return []
        if "select 1 from nodes" in s and "for update" in s:
            tenant_id, trajectory_id, step_n, kind, seq = params
            for row in self.rows.values():
                if (
                    row["tenant_id"] == tenant_id
                    and row["trajectory_id"] == trajectory_id
                    and row["step_n"] == step_n
                    and row["kind"] == kind
                    and row["seq"] == seq
                ):
                    return [(1,)]
            return []
        if s.startswith("select 1 from nodes"):
            # has()
            p_iter = iter(params)
            t_id = next(p_iter)
            sn = next(p_iter)
            k = next(p_iter)
            ten = next(p_iter) if "tenant_id" in s else None
            sq = next(p_iter) if "seq = %s" in s else None
            for row in self.rows.values():
                if row["trajectory_id"] != t_id or row["step_n"] != sn or row["kind"] != k:
                    continue
                if ten is not None and row["tenant_id"] != ten:
                    continue
                if sq is not None and row["seq"] != sq:
                    continue
                return [(1,)]
            return []
        if "select count(*)" in s:
            (node_id,) = params
            return [(1 if node_id in self.rows else 0,)]
        if "select id, trajectory_id, tenant_id" in s:
            trajectory_id = params[0]
            tenant_id = params[1] if len(params) > 1 else None
            out = []
            for row in sorted(
                self.rows.values(),
                key=lambda r: (r["step_n"] if r["step_n"] is not None else -1, r["seq"]),
            ):
                if row["trajectory_id"] != trajectory_id:
                    continue
                if tenant_id is not None and row["tenant_id"] != tenant_id:
                    continue
                out.append(
                    (
                        row["id"],
                        row["trajectory_id"],
                        row["tenant_id"],
                        row["step_n"],
                        row["seq"],
                        row["kind"],
                        row["payload_json"],
                        row["ts"],
                    )
                )
            return out
        return []


class FakePgConnection:
    def __init__(self) -> None:
        self._store = _FakeStore()

    def cursor(self) -> _FakeCursor:
        return _FakeCursor(self._store)

    def commit(self) -> None:
        return None

    def rollback(self) -> None:
        return None

    def close(self) -> None:
        return None


@pytest.fixture
def log() -> PostgresNodeLog:
    return PostgresNodeLog(FakePgConnection())


def test_append_then_has(log: PostgresNodeLog):
    log.append(
        "DECISION",
        step_n=1,
        payload={"plan": "x"},
        trajectory_id="t1",
        tenant_id="demo",
        seq=1,
    )
    assert log.has("t1", "demo", 1, "DECISION")
    assert not log.has("t1", "demo", 1, "TOOL_RESULT")


def test_append_idempotent_by_content(log: PostgresNodeLog):
    n1 = log.append("DECISION", 1, {"plan": "x"}, "t1", "demo", 1)
    n2 = log.append("DECISION", 1, {"plan": "x"}, "t1", "demo", 1)
    assert n1.id == n2.id
    assert log.count(n1.id) == 1


def test_slot_conflict(log: PostgresNodeLog):
    log.append("DECISION", 1, {"plan": "a"}, "t1", "demo", 1)
    with pytest.raises(SlotConflictError):
        log.append("DECISION", 1, {"plan": "b"}, "t1", "demo", 1)


def test_list_nodes_tenant_filter(log: PostgresNodeLog):
    log.append("DECISION", 1, {"plan": "a"}, "t1", "tenant-a", 1)
    log.append("DECISION", 1, {"plan": "b"}, "t1", "tenant-b", 2)
    only_a = log.list_nodes("t1", tenant_id="tenant-a")
    assert len(only_a) == 1
    assert only_a[0]["tenant_id"] == "tenant-a"
    assert json.loads(json.dumps(only_a[0]["payload"])) == {"plan": "a"}


def test_has_tenant_filter(log: PostgresNodeLog):
    # Direct leak regression test: tenant-a writes DECISION, tenant-b must not see it.
    log.append("DECISION", 1, {"plan": "a"}, "t1", "tenant-a", 1)
    assert not log.has("t1", "tenant-b", 1, "DECISION")
    assert log.has("t1", "tenant-a", 1, "DECISION")

    # Multi-tenant slot filtering with seq
    log.append("DECISION", 1, {"plan": "b"}, "t1", "tenant-b", 2)
    assert log.has("t1", "tenant-a", 1, "DECISION")
    assert not log.has("t1", "tenant-a", 1, "DECISION", seq=2)
    assert log.has("t1", "tenant-b", 1, "DECISION", seq=2)
    assert not log.has("t1", "tenant-c", 1, "DECISION")


def test_has_all_tenants(log: PostgresNodeLog):
    log.append("DECISION", 1, {"plan": "a"}, "t1", "tenant-a", 1)
    log.append("DECISION", 1, {"plan": "b"}, "t1", "tenant-b", 2)

    # Scoped check for a third tenant is false
    assert not log.has("t1", "tenant-c", 1, "DECISION")

    # Cross-tenant escape hatch finds nodes from any tenant
    assert log.has_all_tenants("t1", 1, "DECISION")
    assert log.has_all_tenants("t1", 1, "DECISION", seq=2)
    assert not log.has_all_tenants("t1", 1, "DECISION", seq=99)
    assert not log.has_all_tenants("nonexistent-traj", 1, "DECISION")


def test_has_rejects_empty_tenant(log: PostgresNodeLog):
    with pytest.raises(ValueError, match="tenant_id must be a non-empty string"):
        log.has("t1", "", 1, "DECISION")


def test_claim_tool_call_single_winner(log: PostgresNodeLog):
    wins: list[bool] = []

    def worker() -> None:
        claimed = log.claim_tool_call(
            1,
            {"tool": "deploy", "args": {"v": "1"}},
            "t1",
            "demo",
            2,
        )
        wins.append(claimed)

    threads = [threading.Thread(target=worker) for _ in range(8)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    assert sum(1 for w in wins if w) == 1
    assert log.has("t1", "demo", 1, "TOOL_CALL", seq=2)


def test_open_requires_dsn(monkeypatch: pytest.MonkeyPatch):
    monkeypatch.delenv("TRAJIR_DATABASE_URL", raising=False)
    monkeypatch.delenv("DATABASE_URL", raising=False)
    with pytest.raises(ValueError, match="DSN"):
        open_postgres_node_log(None)


@pytest.mark.skipif(
    not os.environ.get("TRAJIR_DATABASE_URL"),
    reason="Set TRAJIR_DATABASE_URL to run live Postgres integration",
)
def test_live_postgres_roundtrip():
    pytest.importorskip("psycopg")
    log = open_postgres_node_log()
    try:
        n = log.append("DECISION", 1, {"plan": "live"}, "live-t", "demo", 1)
        assert log.has("live-t", "demo", 1, "DECISION")
        rows = log.list_nodes("live-t", tenant_id="demo")
        assert rows[0]["id"] == n.id
    finally:
        log.close()
