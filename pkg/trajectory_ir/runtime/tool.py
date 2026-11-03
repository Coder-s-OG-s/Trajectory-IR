from collections.abc import Callable
from dataclasses import dataclass

from trajectory_ir.effects import EffectClass


@dataclass
class Tool:
    name: str
    fn: Callable
    effect_class: EffectClass
    # Required to tag bash/python/sql/browser as PURE, READ_ONLY, or
    # IDEMPOTENT_WRITE. Without it, ExecTool / RunStep refuse the claim.
    allow_open_world_override: bool = False
    # Gated tools only. Status lookup for a call that was claimed but has no
    # recorded outcome; see make_gated_tool_call. Return UNRESOLVED when the
    # host cannot tell, and the gate blocks.
    resolve: Callable | None = None
