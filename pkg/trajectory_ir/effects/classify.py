from enum import Enum


class EffectClass(Enum):
    PURE = "PURE"
    READ_ONLY = "READ_ONLY"
    IDEMPOTENT_WRITE = "IDEMPOTENT_WRITE"
    NON_IDEMPOTENT_WRITE = "NON_IDEMPOTENT_WRITE"
    AGENT_SPAWN = "AGENT_SPAWN"
    SENSITIVE = "SENSITIVE"


def classify_from_mcp(annotations: dict) -> EffectClass:
    """Fail-closed per spec §7.2. Any missing or ambiguous annotation -> NON_IDEMPOTENT_WRITE.

    Contradictory annotations (e.g., readOnlyHint=True AND destructiveHint=True) fail-close to NON_IDEMPOTENT_WRITE.
    """
    if annotations.get("readOnlyHint") is True and annotations.get("destructiveHint") is not True:
        return EffectClass.READ_ONLY
    if (
        annotations.get("readOnlyHint") is False
        and annotations.get("idempotentHint") is True
        and annotations.get("destructiveHint") is False
    ):
        return EffectClass.IDEMPOTENT_WRITE
    return EffectClass.NON_IDEMPOTENT_WRITE  # fail closed, always
