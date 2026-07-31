from trajectory_ir.effects import EffectClass, classify_from_mcp, requires_block_and_gate


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
