from enum import Enum
from typing import Any


class EffectClass(Enum):
    PURE = "PURE"
    READ_ONLY = "READ_ONLY"
    IDEMPOTENT_WRITE = "IDEMPOTENT_WRITE"
    NON_IDEMPOTENT_WRITE = "NON_IDEMPOTENT_WRITE"
    AGENT_SPAWN = "AGENT_SPAWN"
    SENSITIVE = "SENSITIVE"


def requires_block_and_gate(effect: EffectClass) -> bool:
    """Whether resume must gate this effect class (README §8 / R02).

    Only ``NON_IDEMPOTENT_WRITE`` is block-and-gated. ``PURE`` (R03) and other
    non-gated classes may re-execute on resume without raising
    ``BlockedNeedsGate``; durable memoization may still skip re-running the body
    when a prior step result was already recorded.
    """
    return effect is EffectClass.NON_IDEMPOTENT_WRITE


def is_forbidden_in_sandbox(effect: EffectClass) -> bool:
    return effect in (
        EffectClass.NON_IDEMPOTENT_WRITE,
        EffectClass.AGENT_SPAWN,
        EffectClass.SENSITIVE,
    )


# Non-exhaustive. These names are open-world execution primitives: the same
# tool can be a read, a write, a spawn, or a wipe depending on the argument
# string. Static MCP hints on the *tool definition* cannot classify them.
# Hosts must treat them as NON_IDEMPOTENT_WRITE unless an operator sets
# Tool.effect_class explicitly. This is not an AST analyzer.
OPEN_WORLD_PRIMITIVES = frozenset(
    {
        "bash",
        "shell",
        "sh",
        "zsh",
        "terminal",
        "execute",
        "python",
        "python_interpreter",
        "code_interpreter",
        "sql",
        "sql_query",
        "execute_sql",
        "browser",
        "browser_action",
        "computer",
    }
)


def is_open_world_primitive(tool_name: str) -> bool:
    """True for known arbitrary-execution tool names (case-insensitive)."""
    return str(tool_name).strip().lower() in OPEN_WORLD_PRIMITIVES


def classify_tool(tool_name: str, annotations: Any = None) -> EffectClass:
    """Classify a named tool. Open-world primitives fail closed even if MCP
    hints claim they are read-only. Operators override by setting
    ``Tool.effect_class`` directly, not by lying on the hint bits.
    """
    if is_open_world_primitive(tool_name):
        return EffectClass.NON_IDEMPOTENT_WRITE
    return classify_from_mcp(annotations)


def classify_from_mcp(annotations: Any) -> EffectClass:
    """Fail-closed per spec §7.2. Any missing or ambiguous annotation -> NON_IDEMPOTENT_WRITE.

    Contradictory annotations (e.g., readOnlyHint=True AND destructiveHint=True)
    fail-close to NON_IDEMPOTENT_WRITE.

    ``openWorldHint`` alone is never treated as safe: open-world tools without
    an explicit non-destructive read/idempotent write profile fail closed.
    Trust boundary: callers must assign ``Tool.effect_class`` from this mapper
    (or an equivalent operator policy); the runtime does not re-classify
    arbitrary caller labels.
    """
    if not isinstance(annotations, dict):
        return EffectClass.NON_IDEMPOTENT_WRITE

    # openWorldHint=True without a clear safe profile stays fail-closed below.
    if annotations.get("readOnlyHint") is True and annotations.get("destructiveHint") is not True:
        return EffectClass.READ_ONLY
    if (
        annotations.get("readOnlyHint") is False
        and annotations.get("idempotentHint") is True
        and annotations.get("destructiveHint") is False
        and annotations.get("openWorldHint") is not True
    ):
        # openWorld + write is treated as non-idempotent unless operator overrides.
        return EffectClass.IDEMPOTENT_WRITE
    return EffectClass.NON_IDEMPOTENT_WRITE  # fail closed, always
