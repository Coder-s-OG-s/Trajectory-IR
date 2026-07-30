from dataclasses import dataclass
from typing import Callable

from trajectory_ir.effects import EffectClass


@dataclass
class Tool:
    name: str
    fn: Callable
    effect_class: EffectClass
