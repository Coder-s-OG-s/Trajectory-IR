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
