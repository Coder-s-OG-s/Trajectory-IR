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
