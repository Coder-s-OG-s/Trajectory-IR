import pytest

from trajectory_ir.effects import (
    EffectClass,
    OpenWorldOverrideRequired,
    assert_open_world_effect,
    classify_from_mcp,
    classify_tool,
    is_forbidden_in_sandbox,
    is_open_world_primitive,
    requires_block_and_gate,
)


def test_missing_annotations_fail_closed():
    assert classify_from_mcp({}) == EffectClass.NON_IDEMPOTENT_WRITE


def test_ambiguous_annotations_fail_closed():
    assert classify_from_mcp({"readOnlyHint": False}) == EffectClass.NON_IDEMPOTENT_WRITE


def test_read_only_hint_classified_read_only():
    assert classify_from_mcp({"readOnlyHint": True}) == EffectClass.READ_ONLY


def test_explicit_idempotent_write_classified_correctly():
    annotations = {
        "readOnlyHint": False,
        "idempotentHint": True,
        "destructiveHint": False,
    }
    assert classify_from_mcp(annotations) == EffectClass.IDEMPOTENT_WRITE


def test_destructive_hint_true_fails_closed_even_if_idempotent():
    annotations = {
        "readOnlyHint": False,
        "idempotentHint": True,
        "destructiveHint": True,
    }
    assert classify_from_mcp(annotations) == EffectClass.NON_IDEMPOTENT_WRITE


def test_contradictory_readonly_and_destructive_fails_closed():
    annotations = {"readOnlyHint": True, "destructiveHint": True}
    assert classify_from_mcp(annotations) == EffectClass.NON_IDEMPOTENT_WRITE


def test_open_world_idempotent_write_fails_closed():
    annotations = {
        "readOnlyHint": False,
        "idempotentHint": True,
        "destructiveHint": False,
        "openWorldHint": True,
    }
    assert classify_from_mcp(annotations) == EffectClass.NON_IDEMPOTENT_WRITE


def test_non_dict_annotations_fail_closed():
    assert classify_from_mcp(None) == EffectClass.NON_IDEMPOTENT_WRITE  # type: ignore[arg-type]


def test_requires_block_and_gate_only_non_idempotent():
    assert requires_block_and_gate(EffectClass.NON_IDEMPOTENT_WRITE) is True
    assert requires_block_and_gate(EffectClass.PURE) is False
    assert requires_block_and_gate(EffectClass.READ_ONLY) is False
    assert requires_block_and_gate(EffectClass.IDEMPOTENT_WRITE) is False
    assert requires_block_and_gate(EffectClass.AGENT_SPAWN) is False
    assert requires_block_and_gate(EffectClass.SENSITIVE) is False


def test_classify_tool_fail_closes_bash_even_if_readonly_hint():
    read_only = {"readOnlyHint": True}
    assert classify_from_mcp(read_only) == EffectClass.READ_ONLY
    assert classify_tool("bash", read_only) == EffectClass.NON_IDEMPOTENT_WRITE
    assert classify_tool("BASH", read_only) == EffectClass.NON_IDEMPOTENT_WRITE
    assert classify_tool("python_interpreter", read_only) == EffectClass.NON_IDEMPOTENT_WRITE
    assert classify_tool("sql_query", {}) == EffectClass.NON_IDEMPOTENT_WRITE


def test_classify_tool_leaves_ordinary_names_to_mcp_mapping():
    assert classify_tool("echo", {"readOnlyHint": True}) == EffectClass.READ_ONLY
    assert classify_tool("charge_card", {}) == EffectClass.NON_IDEMPOTENT_WRITE


def test_is_open_world_primitive_is_case_insensitive():
    assert is_open_world_primitive("bash") is True
    assert is_open_world_primitive(" Shell ") is True
    assert is_open_world_primitive("echo") is False


def test_assert_open_world_effect_refuses_safer_claim():
    with pytest.raises(OpenWorldOverrideRequired, match="OPEN_WORLD_OVERRIDE_REQUIRED"):
        assert_open_world_effect("bash", EffectClass.READ_ONLY)
    with pytest.raises(OpenWorldOverrideRequired):
        assert_open_world_effect("python", EffectClass.PURE)
    with pytest.raises(OpenWorldOverrideRequired):
        assert_open_world_effect("sql", EffectClass.IDEMPOTENT_WRITE)
    assert_open_world_effect("bash", EffectClass.NON_IDEMPOTENT_WRITE)
    assert_open_world_effect("bash", EffectClass.AGENT_SPAWN)
    assert_open_world_effect("echo", EffectClass.READ_ONLY)
    assert_open_world_effect(
        "bash",
        EffectClass.READ_ONLY,
        allow_override=True,
    )


def test_is_forbidden_in_sandbox():
    assert is_forbidden_in_sandbox(EffectClass.NON_IDEMPOTENT_WRITE) is True
    assert is_forbidden_in_sandbox(EffectClass.AGENT_SPAWN) is True
    assert is_forbidden_in_sandbox(EffectClass.SENSITIVE) is True
    assert is_forbidden_in_sandbox(EffectClass.PURE) is False
    assert is_forbidden_in_sandbox(EffectClass.READ_ONLY) is False
    assert is_forbidden_in_sandbox(EffectClass.IDEMPOTENT_WRITE) is False
