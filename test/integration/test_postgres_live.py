"""Live Postgres NodeLog tests. Require TRAJIR_DATABASE_URL + psycopg."""

from __future__ import annotations

import os
import threading
import uuid

import pytest

pytest.importorskip("psycopg")

from drivers.postgres.log import open_postgres_node_log
from trajectory_ir.runtime.log import SlotConflictError

pytestmark = pytest.mark.skipif(
    not os.environ.get("TRAJIR_DATABASE_URL"),
    reason="Set TRAJIR_DATABASE_URL to run live Postgres integration",
)


@pytest.fixture
def log():
    node_log = open_postgres_node_log()
    yield node_log
    node_log.close()


def test_live_append_has_list_and_idempotent(log):
    traj = f"live-{uuid.uuid4().hex[:12]}"
    n1 = log.append("DECISION", 1, {"plan": "x"}, traj, "demo", 1)
    n2 = log.append("DECISION", 1, {"plan": "x"}, traj, "demo", 1)
    assert n1.id == n2.id
    assert log.has(traj, "demo", 1, "DECISION")
    rows = log.list_nodes(traj, tenant_id="demo")
    assert len(rows) == 1
    assert rows[0]["id"] == n1.id
    assert log.count(n1.id) == 1


def test_live_slot_conflict(log):
    traj = f"live-conflict-{uuid.uuid4().hex[:12]}"
    log.append("DECISION", 1, {"plan": "a"}, traj, "demo", 1)
    with pytest.raises(SlotConflictError):
        log.append("DECISION", 1, {"plan": "b"}, traj, "demo", 1)


def test_live_tenant_isolation(log):
    traj = f"live-tenant-{uuid.uuid4().hex[:12]}"
    log.append("DECISION", 1, {"plan": "a"}, traj, "tenant-a", 1)
    log.append("DECISION", 1, {"plan": "b"}, traj, "tenant-b", 2)
    only_a = log.list_nodes(traj, tenant_id="tenant-a")
    assert len(only_a) == 1
    assert only_a[0]["tenant_id"] == "tenant-a"


def test_live_claim_tool_call_single_winner(log):
    traj = f"live-claim-{uuid.uuid4().hex[:12]}"
    wins: list[bool] = []

    def worker() -> None:
        claimed = log.claim_tool_call(
            1,
            {"tool": "deploy", "args": {"v": "1"}},
            traj,
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
    assert log.has(traj, "demo", 1, "TOOL_CALL", seq=2)
