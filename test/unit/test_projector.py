"""Unit tests for the default context projector (R04)."""

from __future__ import annotations

import pytest

from trajectory_ir.runtime.projector import (
    BudgetImpossible,
    node_size_units,
    project_context,
)


def _node(
    nid: str,
    kind: str,
    payload: dict,
    *,
    step_n: int = 1,
    seq: int = 0,
) -> dict:
    return {
        "id": nid,
        "kind": kind,
        "payload": payload,
        "step_n": step_n,
        "seq": seq,
        "trajectory_id": "t1",
        "tenant_id": "demo",
    }


def test_constraints_always_included_when_under_budget() -> None:
    nodes = [
        _node("c1", "CONSTRAINT", {"rule": "never-drop"}, seq=0),
        _node("t1", "THOUGHT", {"text": "x" * 50}, seq=1),
        _node("c2", "CONSTRAINT", {"rule": "also-keep"}, seq=2),
    ]
    budget = sum(node_size_units(n) for n in nodes)  # room for all
    result = project_context(nodes, budget=budget)
    assert "c1" in result.included_ids
    assert "c2" in result.included_ids
    assert set(result.included_ids) == {"c1", "c2", "t1"}


def test_budget_impossible_when_constraints_alone_exceed() -> None:
    big = {"rule": "x" * 200}
    nodes = [
        _node("c1", "CONSTRAINT", big, seq=0),
        _node("c2", "CONSTRAINT", big, seq=1),
    ]
    must = sum(node_size_units(n) for n in nodes)
    with pytest.raises(BudgetImpossible) as ei:
        project_context(nodes, budget=must - 1)
    assert ei.value.required_size == must
    assert ei.value.budget == must - 1
    assert "BUDGET_IMPOSSIBLE" in str(ei.value)


def test_non_constraints_trimmed_without_dropping_constraints() -> None:
    c = _node("c1", "CONSTRAINT", {"rule": "keep"}, seq=0)
    thoughts = [_node(f"th{i}", "THOUGHT", {"text": "y" * 40}, seq=i + 1) for i in range(5)]
    nodes = [c, *thoughts]
    c_size = node_size_units(c)
    # Budget fits constraint + at most one small thought
    one_thought = node_size_units(thoughts[0])
    budget = c_size + one_thought
    result = project_context(nodes, budget=budget)
    assert "c1" in result.included_ids
    assert result.dropped_ids  # some thoughts dropped
    for did in result.dropped_ids:
        assert did.startswith("th")
    # Never drop the constraint
    assert "c1" not in result.dropped_ids


def test_pinned_ids_always_included() -> None:
    nodes = [
        _node("c1", "CONSTRAINT", {"rule": "a"}, seq=0),
        _node("pin1", "INPUT", {"text": "pinned-input"}, seq=1),
        _node("skip", "THOUGHT", {"text": "z" * 80}, seq=2),
    ]
    pin_size = node_size_units(nodes[0]) + node_size_units(nodes[1])
    result = project_context(nodes, budget=pin_size, pinned_ids={"pin1"})
    assert "c1" in result.included_ids
    assert "pin1" in result.included_ids
    assert "skip" in result.dropped_ids


def test_negative_budget_rejected() -> None:
    with pytest.raises(ValueError, match="budget"):
        project_context([], budget=-1)
