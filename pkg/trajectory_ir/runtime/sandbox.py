"""Sandbox / what-if trajectory mode (README §10 R06).

Live mode is the default. Sandbox mode allows planning and non-gated tools
(PURE, READ_ONLY, etc.) but rejects real NON_IDEMPOTENT_WRITE side effects
before the tool body runs.
"""

from __future__ import annotations

from enum import StrEnum

from trajectory_ir.effects import EffectClass, requires_block_and_gate


class RunMode(StrEnum):
    LIVE = "live"
    SANDBOX = "sandbox"


class SandboxForbidden(Exception):
    """Raised when sandbox mode rejects a tool effect class (R06)."""

    def __init__(self, tool_name: str, effect_class: EffectClass):
        self.tool_name = tool_name
        self.effect_class = effect_class
        super().__init__(
            f"SANDBOX_REJECTS_NON_IDEMPOTENT_WRITE: tool {tool_name!r} "
            f"effect={effect_class.value} is forbidden in sandbox mode"
        )


def normalize_run_mode(mode: RunMode | str | None) -> RunMode:
    if mode is None:
        return RunMode.LIVE
    if isinstance(mode, RunMode):
        return mode
    m = str(mode).strip().lower()
    if m in ("live", ""):
        return RunMode.LIVE
    if m == "sandbox":
        return RunMode.SANDBOX
    raise ValueError(f"unsupported run mode {mode!r}; use live or sandbox")


def assert_tool_allowed_in_mode(
    mode: RunMode | str | None,
    *,
    tool_name: str,
    effect_class: EffectClass,
) -> None:
    """Raise SandboxForbidden if this tool must not run under the given mode."""
    run_mode = normalize_run_mode(mode)
    if run_mode is RunMode.SANDBOX and requires_block_and_gate(effect_class):
        raise SandboxForbidden(tool_name, effect_class)
