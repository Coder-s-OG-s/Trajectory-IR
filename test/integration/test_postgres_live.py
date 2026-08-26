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


@pytest.fixture
def pool_log():
    node_log = open_postgres_node_log(pool_size=5)
    yield node_log
    node_log.close()


def test_live_pool_concurrency(pool_log):
    traj = f"live-pool-{uuid.uuid4().hex[:12]}"

    # We will spawn 10 threads. Each thread tries to claim the same TOOL_CALL slot,
    # and then appends a unique node. We expect exactly one thread to successfully
    # claim the tool call, and all threads to successfully append their unique nodes.

    success_claims = []
    append_ids = []
    errors = []

    def worker(i: int):
        try:
            # Concurrent claims
            claimed = pool_log.claim_tool_call(
                step_n=1, payload={"tool": "test"}, trajectory_id=traj, tenant_id="demo", seq=1
            )
            if claimed:
                success_claims.append(i)

            # Concurrent unique appends (different seq)
            node = pool_log.append(
                kind="TOOL_RESULT",
                step_n=1,
                payload={"result": i},
                trajectory_id=traj,
                tenant_id="demo",
                seq=10 + i,
            )
            append_ids.append(node.id)
        except Exception as e:
            errors.append(e)

    threads = [threading.Thread(target=worker, args=(i,)) for i in range(10)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()

    assert not errors, f"Concurrent workers hit exceptions: {errors}"
    assert len(success_claims) == 1, "Exactly one thread should win the claim_tool_call"

    rows = pool_log.list_nodes(traj, tenant_id="demo")

    # 1 claim + 10 appends = 11 nodes total
    assert len(rows) == 11
