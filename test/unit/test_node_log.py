import os
import tempfile
import threading

import pytest

from trajectory_ir.runtime.log import NodeLog, SlotConflictError


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
