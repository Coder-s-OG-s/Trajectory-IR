"""Tests for file based projector policy."""

from __future__ import annotations

from pathlib import Path

import pytest

from trajectory_ir.runtime.policy import (
    PolicyError,
    ProjectorPolicy,
    load_projector_policy,
    policy_from_mapping,
)
from trajectory_ir.runtime.projector import project_context


def _node(nid: str, kind: str, payload: dict, *, seq: int = 0) -> dict:
    return {
        "id": nid,
        "kind": kind,
        "payload": payload,
        "step_n": 1,
        "seq": seq,
    }


def test_load_yaml_subset(tmp_path: Path) -> None:
    p = tmp_path / "policy.yaml"
    p.write_text(
        "default_budget: 1000\n"
        "always_include_kinds:\n"
        "  - CONSTRAINT\n"
        "  - DECISION\n"
        "size_metric: rfc8785_bytes\n",
        encoding="utf-8",
    )
    pol = load_projector_policy(p)
    assert pol.default_budget == 1000
    assert pol.always_include_kinds == frozenset({"CONSTRAINT", "DECISION"})


def test_load_json(tmp_path: Path) -> None:
    p = tmp_path / "policy.json"
    p.write_text(
        '{"default_budget": 200, "always_include_kinds": ["CONSTRAINT"]}',
        encoding="utf-8",
    )
    pol = load_projector_policy(p)
    assert pol.default_budget == 200


def test_unknown_key_fails(tmp_path: Path) -> None:
    p = tmp_path / "bad.yaml"
    p.write_text("default_budget: 1\nexotic_plugin: true\n", encoding="utf-8")
    with pytest.raises(PolicyError, match="unknown"):
        load_projector_policy(p)


def test_bad_metric_fails() -> None:
    with pytest.raises(PolicyError, match="size_metric"):
        policy_from_mapping({"size_metric": "tokens", "always_include_kinds": ["CONSTRAINT"]})


def test_project_uses_policy_kinds() -> None:
    nodes = [
        _node("d1", "DECISION", {"plan": {}}, seq=0),
        _node("t1", "THOUGHT", {"text": "x" * 20}, seq=1),
    ]
    pol = ProjectorPolicy(always_include_kinds=frozenset({"DECISION"}), default_budget=10_000)
    result = project_context(nodes, policy=pol)
    assert "d1" in result.included_ids


def test_budget_from_policy_when_arg_omitted() -> None:
    nodes = [_node("c1", "CONSTRAINT", {"rule": "a"}, seq=0)]
    pol = ProjectorPolicy(default_budget=50_000)
    result = project_context(nodes, policy=pol)
    assert result.budget == 50_000
    assert "c1" in result.included_ids


def test_missing_budget_errors() -> None:
    nodes = [_node("c1", "CONSTRAINT", {"rule": "a"}, seq=0)]
    with pytest.raises(ValueError, match="budget"):
        project_context(nodes, policy=ProjectorPolicy())


def test_repo_example_policy_loads() -> None:
    root = Path(__file__).resolve().parents[2]
    pol = load_projector_policy(root / "testdata" / "projector_policy_default.yaml")
    assert pol.default_budget == 50000
    assert "CONSTRAINT" in pol.always_include_kinds
