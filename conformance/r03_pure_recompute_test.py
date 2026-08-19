"""Conformance test for R03: PURE tools may recompute freely on resume.

README §10 R03 — A PURE tool may be recomputed freely on resume.

This is the dual of R02: NON_IDEMPOTENT_WRITE is block-and-gated, while PURE
must never raise BlockedNeedsGate solely because a prior TOOL_CALL exists for
the same step/seq.
"""

from __future__ import annotations

import pytest

from trajectory_ir.effects import EffectClass, requires_block_and_gate
from trajectory_ir.resume.gate import BlockedNeedsGate, make_gated_tool_call
from trajectory_ir.runtime.log import NodeLog

# Module-level callables: DBOS pickles workflow args (same pattern as kill-mid-deploy).
_R03_CALLS = {"n": 0}


def _r03_compute(x: int = 0) -> int:
    _R03_CALLS["n"] += 1
    return _R03_CALLS["n"]


def _r03_model_call(context: dict) -> dict:
    return {"tool_calls": [{"name": "compute", "args": {"x": 1}}]}


def test_r03_requires_block_and_gate_matrix() -> None:
    """Only NON_IDEMPOTENT_WRITE is gated; PURE is not (R02 vs R03)."""
    assert requires_block_and_gate(EffectClass.NON_IDEMPOTENT_WRITE) is True
    assert requires_block_and_gate(EffectClass.PURE) is False
    assert requires_block_and_gate(EffectClass.READ_ONLY) is False
    assert requires_block_and_gate(EffectClass.IDEMPOTENT_WRITE) is False


def test_r03_pure_tool_may_recompute_after_prior_tool_call(tmp_path) -> None:
    """R03: a prior TOOL_CALL for a PURE tool does not block recompute.

    Simulate history with TOOL_CALL at seq=2, then re-run the PURE body (as the
    non-gated path would). Counter must increase twice; BlockedNeedsGate must
    not fire on the pure path. The same history under the gate still blocks (R02).
    """
    traj, tenant = "r03-pure", "demo"
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        log.append(
            "TOOL_CALL",
            1,
            {"tool": "compute", "args": {}},
            traj,
            tenant,
            2,
        )

        calls = {"n": 0}

        def compute() -> int:
            calls["n"] += 1
            return calls["n"]

        # Non-gated path (R03 / make_run_step else branch): call body freely.
        assert compute() == 1
        assert compute() == 2
        assert calls["n"] == 2

        # Contrast R02: the same history under the gate would block.
        gated = make_gated_tool_call(log, traj, tenant, 1, 2, "compute", compute)
        with pytest.raises(BlockedNeedsGate) as ei:
            gated()
        assert ei.value.step_n == 1
        assert ei.value.tool_name == "compute"
        assert calls["n"] == 2
        assert log.has(traj, tenant, 1, "ABORT", seq=3)
    finally:
        log.close()


def test_r03_pure_run_step_does_not_block_on_second_invocation(tmp_path, monkeypatch) -> None:
    """R03 via make_run_step: two durable steps with PURE never raise the gate."""
    from dbos import SetWorkflowID

    from drivers.durable_backend.dbos.adapter import init_backend
    from trajectory_ir.resume.step import make_run_step
    from trajectory_ir.runtime.tool import Tool

    monkeypatch.chdir(tmp_path)
    traj, tenant = "r03-run-step", "demo"
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    _R03_CALLS["n"] = 0

    registry = {
        "compute": Tool(name="compute", fn=_r03_compute, effect_class=EffectClass.PURE),
    }
    run_step = make_run_step(log, tenant, traj, registry)
    init_backend(app_name="r03-pure-recompute")

    with SetWorkflowID(f"{traj}-a"):
        r1 = run_step(step_n=1, model_call=_r03_model_call, context={})
    with SetWorkflowID(f"{traj}-b"):
        r2 = run_step(step_n=2, model_call=_r03_model_call, context={})

    assert r1 == [1]
    assert r2 == [2]
    assert _R03_CALLS["n"] == 2
    assert log.has(traj, tenant, 1, "TOOL_CALL", seq=2)
    assert log.has(traj, tenant, 2, "TOOL_CALL", seq=2)
    assert not log.has(traj, tenant, 1, "ABORT", seq=3)
    assert not log.has(traj, tenant, 2, "ABORT", seq=3)
