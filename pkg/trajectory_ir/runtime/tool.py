from collections.abc import Callable
from dataclasses import dataclass

from trajectory_ir.effects import EffectClass


@dataclass
class Tool:
    name: str
    fn: Callable
    effect_class: EffectClass
