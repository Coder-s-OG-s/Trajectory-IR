import os
import sqlite3
import tempfile
import threading

import pytest

from trajectory_ir.runtime.log import NodeLog, NodeNotStoredError, SlotConflictError


@pytest.fixture
def log():
    fd, path = tempfile.mkstemp(suffix=".sqlite")
    os.close(fd)
    node_log = NodeLog(path)
    yield node_log
    node_log.close()
    os.remove(path)


def test_append_then_has(log):
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


def test_append_is_idempotent_by_content(log):
    n1 = log.append(
        "DECISION",
        step_n=1,
        payload={"plan": "x"},
        trajectory_id="t1",
        tenant_id="demo",
        seq=1,
    )
    n2 = log.append(
        "DECISION",
        step_n=1,
        payload={"plan": "x"},
        trajectory_id="t1",
        tenant_id="demo",
        seq=1,
    )
    assert n1.id == n2.id
    assert log.count(n1.id) == 1


def test_claim_tool_call_atomic_single_winner(log):
    wins = []

    def worker():
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
    assert sum(1 for w in wins if not w) == 7
    assert log.has("t1", "demo", 1, "TOOL_CALL", seq=2)


def test_list_nodes_tenant_filter(log):
    log.append("DECISION", 1, {"plan": "a"}, "t1", "tenant-a", 1)
    log.append("DECISION", 1, {"plan": "b"}, "t1", "tenant-b", 2)
    only_a = log.list_nodes("t1", tenant_id="tenant-a")
    assert len(only_a) == 1
    assert only_a[0]["tenant_id"] == "tenant-a"
    all_nodes = log.list_nodes_all_tenants("t1")
    assert len(all_nodes) == 2


def test_list_nodes_page(log):
    for seq in range(5):
        log.append("THOUGHT", 1, {"i": seq}, "t1", "demo", seq)
    page = log.list_nodes("t1", tenant_id="demo", limit=2, offset=1)
    assert [row["seq"] for row in page] == [1, 2]
    assert [row["payload"]["i"] for row in page] == [1, 2]
    assert log.list_nodes("t1", tenant_id="demo", limit=2, offset=10) == []
    assert log.list_nodes("t1", tenant_id="demo", limit=0) == []
    assert [row["seq"] for row in log.list_nodes("t1", tenant_id="demo")] == [0, 1, 2, 3, 4]


def test_list_nodes_rejects_bad_page(log):
    with pytest.raises(ValueError, match="limit"):
        log.list_nodes("t1", tenant_id="demo", limit=-1)
    with pytest.raises(ValueError, match="offset"):
        log.list_nodes("t1", tenant_id="demo", offset=-1)
    with pytest.raises(ValueError, match="limit"):
        log.list_nodes("t1", tenant_id="demo", limit=True)


def test_has_trajectory(log):
    assert log.has_trajectory("t1", tenant_id="demo") is False
    log.append("THOUGHT", 1, {"i": 1}, "t1", "demo", 0)
    log.append("THOUGHT", 1, {"i": 2}, "t1", "other", 1)
    assert log.has_trajectory("t1", tenant_id="demo") is True
    assert log.has_trajectory("t1", tenant_id="other") is True
    assert log.has_trajectory("missing", tenant_id="demo") is False
    with pytest.raises(ValueError, match="tenant_id"):
        log.has_trajectory("t1", tenant_id="")


def test_get_kind_payload_reads_lowest_seq(log):
    for seq in range(4):
        log.append("THOUGHT", 1, {"i": seq}, "t1", "demo", seq)
    log.append(
        "DECISION",
        2,
        {"plan": {"n": 9}, "world_snapshot": {"k": "later"}},
        "t1",
        "demo",
        4,
    )
    log.append(
        "DECISION",
        2,
        {"plan": {"n": 1}, "world_snapshot": {"k": "v"}},
        "t1",
        "demo",
        1,
    )
    got = log.get_kind_payload("t1", "demo", 2, "DECISION")
    assert got == {"plan": {"n": 1}, "world_snapshot": {"k": "v"}}
    assert log.get_kind_payload("t1", "demo", 1, "DECISION") is None


def test_file_log_uses_wal(log):
    mode = log._conn.execute("PRAGMA journal_mode").fetchone()[0]
    assert mode == "wal"


def test_node_log_opens_beside_an_existing_wal_connection(tmp_path):
    path = tmp_path / "shared.sqlite"
    setup = sqlite3.connect(path)
    setup.execute("PRAGMA journal_mode=WAL")
    setup.close()
    holder = sqlite3.connect(path)
    try:
        log = NodeLog(str(path))
        try:
            mode = log._conn.execute("PRAGMA journal_mode").fetchone()[0]
            assert mode == "wal"
        finally:
            log.close()
    finally:
        holder.close()


def test_append_errors_when_slot_owner_missing(log):
    node = log.append("THOUGHT", 1, {"text": "hi"}, "traj", "tenant", 0)
    log._conn.execute("UPDATE nodes SET step_n = 99 WHERE id = ?", (node.id,))
    log._conn.commit()
    with pytest.raises(NodeNotStoredError):
        log.append("THOUGHT", 1, {"text": "hi"}, "traj", "tenant", 0)
    row = log._conn.execute(
        "SELECT step_n FROM nodes WHERE id = ?",
        (node.id,),
    ).fetchone()
    assert row == (99,)


def test_slot_conflict_different_payload(log):
    log.append("DECISION", 1, {"plan": "a"}, "t1", "demo", 1)
    with pytest.raises(SlotConflictError):
        log.append("DECISION", 1, {"plan": "b"}, "t1", "demo", 1)


def test_has_tenant_filter(log):
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


def test_has_all_tenants(log):
    log.append("DECISION", 1, {"plan": "a"}, "t1", "tenant-a", 1)
    log.append("DECISION", 1, {"plan": "b"}, "t1", "tenant-b", 2)

    # Scoped check for a third tenant is false
    assert not log.has("t1", "tenant-c", 1, "DECISION")

    # Cross-tenant escape hatch finds nodes from any tenant
    assert log.has_all_tenants("t1", 1, "DECISION")
    assert log.has_all_tenants("t1", 1, "DECISION", seq=2)
    assert not log.has_all_tenants("t1", 1, "DECISION", seq=99)
    assert not log.has_all_tenants("nonexistent-traj", 1, "DECISION")


def test_has_rejects_empty_tenant(log):
    with pytest.raises(ValueError, match="tenant_id must be a non-empty string"):
        log.has("t1", "", 1, "DECISION")
