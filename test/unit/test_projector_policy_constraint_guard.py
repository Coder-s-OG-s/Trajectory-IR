from trajectory_ir.runtime.policy import ProjectorPolicy, policy_from_mapping


def test_direct_construction_without_constraint_still_includes_it():
    policy = ProjectorPolicy(always_include_kinds=frozenset({"DECISION"}))
    assert "CONSTRAINT" in policy.always_include_kinds
    assert "DECISION" in policy.always_include_kinds


def test_policy_from_mapping_without_constraint_still_includes_it():
    policy = policy_from_mapping({"always_include_kinds": ["DECISION"]})
    assert "CONSTRAINT" in policy.always_include_kinds
    assert "DECISION" in policy.always_include_kinds


def test_policy_from_mapping_explicit_constraint_unaffected():
    policy = policy_from_mapping({"always_include_kinds": ["CONSTRAINT", "DECISION"]})
    assert policy.always_include_kinds == frozenset({"CONSTRAINT", "DECISION"})
