import time

import pytest

from trajectory_ir.runtime.nodes import Node, NodeValidationError, node_id


def test_identical_payload_different_key_order_same_hash():
    n1 = Node(
        kind="STATE_SET",
        trajectory_id="t1",
        tenant_id="demo",
        step_n=1,
        seq=1,
        payload={"a": 1, "b": 2},
    )
    n2 = Node(
        kind="STATE_SET",
        trajectory_id="t1",
        tenant_id="demo",
        step_n=1,
        seq=1,
        payload={"b": 2, "a": 1},
    )
    assert n1.id == n2.id  # whole point of JCS


def test_ts_never_affects_hash():
    n1 = Node(
        kind="STATE_SET",
        trajectory_id="t1",
        tenant_id="demo",
        step_n=1,
        seq=1,
        payload={"a": 1},
    )
    time.sleep(1.1)
    n2 = Node(
        kind="STATE_SET",
        trajectory_id="t1",
        tenant_id="demo",
        step_n=1,
        seq=1,
        payload={"a": 1},
    )
    assert n1.id == n2.id  # ts differs, id must not


def test_unknown_kind_rejected():
    with pytest.raises(NodeValidationError):
        Node(
            kind="NOT_A_KIND",
            trajectory_id="t1",
            tenant_id="demo",
            step_n=1,
            seq=1,
            payload={},
        )


def test_ts_key_in_payload_rejected():
    with pytest.raises(NodeValidationError):
        Node(
            kind="STATE_SET",
            trajectory_id="t1",
            tenant_id="demo",
            step_n=1,
            seq=1,
            payload={"ts": 123},
        )


def test_pipe_in_tenant_id_rejected():
    with pytest.raises(NodeValidationError, match="delimiter"):
        Node(
            kind="STATE_SET",
            trajectory_id="t1",
            tenant_id="a|b",
            step_n=1,
            seq=1,
            payload={"a": 1},
        )


def test_pipe_in_trajectory_id_rejected():
    with pytest.raises(NodeValidationError, match="delimiter"):
        Node(
            kind="STATE_SET",
            trajectory_id="x|y",
            tenant_id="demo",
            step_n=1,
            seq=1,
            payload={"a": 1},
        )


def test_pipe_in_kind_rejected_by_node_id():
    with pytest.raises(NodeValidationError, match="delimiter"):
        node_id("demo", "t1", 1, 0, "TOOL|CALL", "ab" * 32)


@pytest.mark.parametrize("step_n", ["3|1", "3", 3.0, True])
def test_non_int_step_n_rejected_by_node_id(step_n):
    with pytest.raises(NodeValidationError, match="step_n"):
        node_id("acme", "demo", step_n, 0, "DECISION", "a" * 64)


@pytest.mark.parametrize("seq", ["0|X", "0", 0.0, False])
def test_non_int_seq_rejected_by_node_id(seq):
    with pytest.raises(NodeValidationError, match="seq"):
        node_id("acme", "demo", 3, seq, "DECISION", "a" * 64)


def test_pipe_in_payload_hash_rejected_by_node_id():
    with pytest.raises(NodeValidationError, match="delimiter"):
        node_id("acme", "demo", 3, 0, "DECISION", "a|b")


def test_int_and_none_step_n_still_accepted():
    node_id("acme", "demo", 3, 0, "DECISION", "a" * 64)
    node_id("acme", "demo", None, 0, "DECISION", "a" * 64)
