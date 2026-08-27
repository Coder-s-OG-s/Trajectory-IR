"""Conformance test for R04: CONSTRAINT never silently dropped under budget.

README §10 R04 — A CONSTRAINT node is never silently dropped under budget
pressure; the system either includes it or raises a hard error.
"""

from __future__ import annotations

import pytest

pytest.importorskip("rfc8785")

from trajectory_ir.runtime.projector import (
    BudgetImpossible,
    node_size_units,
    project_context,
)


def _node(nid: str, kind: str, payload: dict, *, seq: int = 0) -> dict:
    return {
        "id": nid,
        "kind": kind,
        "payload": payload,
        "step_n": 1,
        "seq": seq,
        "trajectory_id": "r04",
        "tenant_id": "demo",
    }


def test_r04_all_constraints_included_when_budget_allows() -> None:
    nodes = [
        _node("c-a", "CONSTRAINT", {"rule": "A"}, seq=0),
        _node("c-b", "CONSTRAINT", {"rule": "B"}, seq=1),
        _node("thought", "THOUGHT", {"text": "optional"}, seq=2),
    ]
    budget = sum(node_size_units(n) for n in nodes)
    result = project_context(nodes, budget=budget)
    assert "c-a" in result.included_ids
    assert "c-b" in result.included_ids
    assert set(result.context["included_ids"]) == set(result.included_ids)


def test_r04_budget_impossible_never_partial_drops_constraints() -> None:
    """If constraints alone exceed budget, fail hard — do not return a subset."""
    payload = {"rule": "long-" + ("Z" * 300)}
    nodes = [
        _node("c1", "CONSTRAINT", payload, seq=0),
        _node("c2", "CONSTRAINT", payload, seq=1),
    ]
    must = sum(node_size_units(n) for n in nodes)
    with pytest.raises(BudgetImpossible) as ei:
        project_context(nodes, budget=max(1, must // 2))
    assert ei.value.required_size > ei.value.budget
    assert "BUDGET_IMPOSSIBLE" in str(ei.value)


def test_r04_trims_optional_nodes_but_keeps_every_constraint() -> None:
    constraints = [
        _node("c1", "CONSTRAINT", {"rule": "must"}, seq=0),
        _node("c2", "CONSTRAINT", {"rule": "also"}, seq=1),
    ]
    optional = [_node(f"o{i}", "STATE_SET", {"k": "v" * 30}, seq=10 + i) for i in range(8)]
    nodes = constraints + optional
    c_size = sum(node_size_units(n) for n in constraints)
    # Enough for both constraints + zero or one optional
    budget = c_size + node_size_units(optional[0])
    result = project_context(nodes, budget=budget)
    assert "c1" in result.included_ids and "c2" in result.included_ids
    for cid in ("c1", "c2"):
        assert cid not in result.dropped_ids
    assert any(d.startswith("o") for d in result.dropped_ids)
