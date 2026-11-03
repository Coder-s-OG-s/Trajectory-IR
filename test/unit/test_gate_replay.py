import pickle

import pytest

from trajectory_ir.client.python.trajectory_client import exec_tool, open_trajectory
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import (
    UNRESOLVED,
    BlockedNeedsGate,
    ToolCallConflict,
    make_gated_tool_call,
)
from trajectory_ir.resume.idempotency import CallMeta, current_idempotency_key
from trajectory_ir.runtime.log import NodeLog, SlotConflictError
from trajectory_ir.runtime.tool import Tool

TENANT = "demo"
TRAJ = "replay"


@pytest.fixture
def node_log(tmp_path):
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    yield log
    log.close()


def _claim(node_log, tool="spawn_builder", args=None):
    meta = CallMeta.make(TENANT, TRAJ, 1, 2)
    assert node_log.claim_tool_call(
        1,
        {"tool": tool, "args": args or {}, "idempotency_key": meta.idempotency_key},
        TRAJ,
        TENANT,
        2,
    )


def test_replay_with_different_args_raises_conflict(node_log):
    runs = []

    def charge(amount):
        runs.append(amount)
        return {"charged": amount}

    gated = make_gated_tool_call(node_log, TRAJ, TENANT, 1, 2, "charge", charge)
    assert gated(amount=10) == {"charged": 10}

    with pytest.raises(ToolCallConflict) as info:
        gated(amount=20)
    assert isinstance(info.value, SlotConflictError)
    assert (info.value.step_n, info.value.seq, info.value.tool_name) == (1, 2, "charge")
    assert runs == [10]
    assert not node_log.has(TRAJ, TENANT, 1, "ABORT", seq=3)


def test_replay_with_different_tool_name_raises_conflict(node_log):
    make_gated_tool_call(node_log, TRAJ, TENANT, 1, 2, "charge", lambda: "ok")()

    def refund():
        raise AssertionError("refund must not run")

    with pytest.raises(ToolCallConflict):
        make_gated_tool_call(node_log, TRAJ, TENANT, 1, 2, "refund", refund)()


def test_replay_matches_args_by_canonical_form(node_log):
    gated = make_gated_tool_call(node_log, TRAJ, TENANT, 1, 2, "charge", lambda amount: amount)
    assert gated(amount=10) == 10
    # 10.0 and 10 hash the same under RFC 8785, as they do in the node id.
    assert gated(amount=10.0) == 10


def test_interrupted_call_with_different_args_conflicts_before_abort(node_log):
    _claim(node_log, args={"task": "build"})
    gated = make_gated_tool_call(node_log, TRAJ, TENANT, 1, 2, "spawn_builder", lambda task: task)
    with pytest.raises(ToolCallConflict):
        gated(task="deploy")
    assert not node_log.has(TRAJ, TENANT, 1, "ABORT", seq=3)


def test_tool_call_conflict_survives_pickle():
    err = pickle.loads(pickle.dumps(ToolCallConflict(3, 4, "charge")))
    assert (err.step_n, err.seq, err.tool_name) == (3, 4, "charge")


def test_resolve_records_result_instead_of_blocking(node_log):
    _claim(node_log, args={"task": "build"})
    seen = []

    def resolve(task):
        seen.append((task, current_idempotency_key()))
        return {"child": "done"}

    def spawn(task):
        raise AssertionError("spawn must not run again")

    gated = make_gated_tool_call(
        node_log, TRAJ, TENANT, 1, 2, "spawn_builder", spawn, resolve_fn=resolve
    )
    assert gated(task="build") == {"child": "done"}
    assert seen == [("build", CallMeta.make(TENANT, TRAJ, 1, 2).idempotency_key)]
    assert node_log.get_node_payload(TRAJ, TENANT, 1, 3, "TOOL_RESULT") == {
        "result": {"child": "done"}
    }
    assert not node_log.has(TRAJ, TENANT, 1, "ABORT", seq=3)

    # The recorded result answers later re-entry without asking resolve again.
    plain = make_gated_tool_call(node_log, TRAJ, TENANT, 1, 2, "spawn_builder", spawn)
    assert plain(task="build") == {"child": "done"}
    assert len(seen) == 1


def test_unresolved_blocks_then_resolves_on_a_later_reentry(node_log):
    _claim(node_log, args={"task": "build"})
    state = {"finished": False}

    def resolve(task):
        return "child-ok" if state["finished"] else UNRESOLVED

    def spawn(task):
        raise AssertionError("spawn must not run again")

    gated = make_gated_tool_call(
        node_log, TRAJ, TENANT, 1, 2, "spawn_builder", spawn, resolve_fn=resolve
    )
    with pytest.raises(BlockedNeedsGate):
        gated(task="build")
    assert node_log.has(TRAJ, TENANT, 1, "ABORT", seq=3)

    state["finished"] = True
    assert gated(task="build") == "child-ok"


def test_resolve_error_propagates_without_abort(node_log):
    _claim(node_log)

    def resolve():
        raise ConnectionError("status backend down")

    gated = make_gated_tool_call(
        node_log, TRAJ, TENANT, 1, 2, "spawn_builder", lambda: None, resolve_fn=resolve
    )
    with pytest.raises(ConnectionError):
        gated()
    assert not node_log.has(TRAJ, TENANT, 1, "ABORT", seq=3)


def test_exec_tool_passes_tool_resolve_to_the_gate(tmp_path):
    def spawn(task):
        raise AssertionError("spawn must not run again")

    tool = Tool(
        name="spawn_builder",
        fn=spawn,
        effect_class=EffectClass.AGENT_SPAWN,
        resolve=lambda task: {"child": task},
    )
    trajectory = open_trajectory(TENANT, TRAJ, db_path=str(tmp_path / "t.sqlite"))
    _claim(trajectory._log, args={"task": "build"})

    out = exec_tool(trajectory, step_n=1, call={"args": {"task": "build"}}, tool=tool, seq=2)
    assert out.result == {"child": "build"}


def test_sensitive_tool_is_gated_on_reentry(tmp_path):
    rotated = []

    def rotate_key(key_id):
        rotated.append(key_id)
        return {"rotated": key_id}

    tool = Tool(name="rotate_key", fn=rotate_key, effect_class=EffectClass.SENSITIVE)
    trajectory = open_trajectory(TENANT, TRAJ, db_path=str(tmp_path / "t.sqlite"))
    _claim(trajectory._log, tool="rotate_key", args={"key_id": "k1"})

    with pytest.raises(BlockedNeedsGate):
        exec_tool(trajectory, step_n=1, call={"args": {"key_id": "k1"}}, tool=tool, seq=2)
    assert rotated == []


def test_get_node_payload_reads_one_slot(node_log):
    node_log.append("TOOL_CALL", 1, {"seq": 2}, TRAJ, TENANT, 2)
    node_log.append("TOOL_RESULT", 1, {"seq": 3}, TRAJ, TENANT, 3)

    assert node_log.get_node_payload(TRAJ, TENANT, 1, 3, "TOOL_RESULT") == {"seq": 3}
    assert node_log.get_node_payload(TRAJ, TENANT, 1, 3, "TOOL_CALL") is None
    assert node_log.get_node_payload(TRAJ, TENANT, 1, 5, "TOOL_RESULT") is None
    assert node_log.get_node_payload(TRAJ, "other", 1, 3, "TOOL_RESULT") is None
    with pytest.raises(ValueError):
        node_log.get_node_payload(TRAJ, "", 1, 3, "TOOL_RESULT")
