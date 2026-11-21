"""Live Postgres NodeLog tests. Require TRAJIR_DATABASE_URL + psycopg."""

from __future__ import annotations

import os
import threading
import uuid

import pytest

pytest.importorskip("psycopg")

from trajectory_ir.drivers.postgres.log import open_postgres_node_log
from trajectory_ir.package import export_tir, import_tir
from trajectory_ir.runtime.log import NodeLog, SlotConflictError

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


def test_live_append_many_commits_batch(log):
    traj = f"live-batch-{uuid.uuid4().hex[:12]}"
    rows = [
        ("PROJECT_CONTEXT", 1, {"goal": "batch"}, traj, "demo", 0),
        ("DECISION", 1, {"plan": "first"}, traj, "demo", 1),
        ("THOUGHT", 1, {"text": "second"}, traj, "demo", 2),
    ]

    log.append_many(rows)

    stored = log.list_nodes(traj, tenant_id="demo")
    assert len(stored) == len(rows)
    assert [node["seq"] for node in stored] == [0, 1, 2]
    assert [node["kind"] for node in stored] == ["PROJECT_CONTEXT", "DECISION", "THOUGHT"]


def test_live_append_many_rolls_back_conflicting_batch_and_recovers(log):
    traj = f"live-batch-conflict-{uuid.uuid4().hex[:12]}"
    log.append("DECISION", 1, {"plan": "existing"}, traj, "demo", 1)

    with pytest.raises(SlotConflictError):
        log.append_many(
            [
                ("PROJECT_CONTEXT", 1, {"goal": "must roll back"}, traj, "demo", 0),
                ("DECISION", 1, {"plan": "conflicting"}, traj, "demo", 1),
                ("THOUGHT", 1, {"text": "must not be written"}, traj, "demo", 2),
            ]
        )

    after_failure = log.list_nodes(traj, tenant_id="demo")
    assert len(after_failure) == 1
    assert after_failure[0]["kind"] == "DECISION"
    assert after_failure[0]["payload"] == {"plan": "existing"}

    # The failed transaction must not leave the connection in PostgreSQL's
    # aborted-transaction state.
    recovered = log.append("THOUGHT", 1, {"text": "connection recovered"}, traj, "demo", 3)
    assert log.count(recovered.id) == 1


def test_live_append_many_is_idempotent_for_reimport(log, tmp_path):
    traj = f"live-reimport-{uuid.uuid4().hex[:12]}"
    source = NodeLog(str(tmp_path / "source.sqlite"))
    try:
        source.append("PROJECT_CONTEXT", 1, {"goal": "reimport"}, traj, "demo", 0)
        source.append("DECISION", 1, {"plan": {"tool_calls": []}}, traj, "demo", 1)
        package = tmp_path / "reimport.tir"
        export_tir(source, traj, package, mode="thin")
    finally:
        source.close()

    import_tir(package, log)
    first_import = log.list_nodes(traj, tenant_id="demo")
    import_tir(package, log)
    second_import = log.list_nodes(traj, tenant_id="demo")

    assert len(first_import) == 2
    assert [node["id"] for node in second_import] == [node["id"] for node in first_import]


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
