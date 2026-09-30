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


# Effect classes that claim to be safer than an open-world primitive can be.
# NON_IDEMPOTENT_WRITE is the honest default. AGENT_SPAWN and SENSITIVE are
# not "safer" claims; ExecTool still allows them without an override.
_SAFER_THAN_OPEN_WORLD = frozenset(
    {
        EffectClass.PURE,
        EffectClass.READ_ONLY,
        EffectClass.IDEMPOTENT_WRITE,
    }
)


class OpenWorldOverrideRequired(Exception):
    """Raised when an open-world primitive is tagged safer than it is."""

    def __init__(self, tool_name: str, effect_class: EffectClass):
        self.tool_name = tool_name
        self.effect_class = effect_class
        super().__init__(
            f"OPEN_WORLD_OVERRIDE_REQUIRED: tool {tool_name!r} is an "
            f"open-world primitive tagged {effect_class.value}; set "
            "allow_open_world_override or classify it NON_IDEMPOTENT_WRITE"
        )


def assert_open_world_effect(
    tool_name: str,
    effect: EffectClass,
    *,
    allow_override: bool = False,
) -> None:
    """Refuse open-world names tagged PURE/READ_ONLY/IDEMPOTENT_WRITE.

    ``ClassifyTool`` only covers the mapper. ExecTool / RunStep must still
    reject a Tool struct that claims bash is read-only unless the operator
    set ``allow_open_world_override`` and owns that lie. This is not an AST.
    """
    if not is_open_world_primitive(tool_name):
        return
    if effect not in _SAFER_THAN_OPEN_WORLD:
        return
    if allow_override:
        return
    raise OpenWorldOverrideRequired(tool_name, effect)


def classify_tool(tool_name: str, annotations: Any = None) -> EffectClass:
    """Classify a named tool. Open-world primitives fail closed even if MCP
    hints claim they are read-only. Operators who still want a safer class
    must set ``Tool.effect_class`` *and* ``allow_open_world_override``.
    Hint bits alone are not enough, and neither is the Tool field without
    the override flag. This is not an AST analyzer.
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
