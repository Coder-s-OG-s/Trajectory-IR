"""File based projector policy (narrow config, not a full DSL).

Default behavior without a file matches the built in R04 policy: always
include CONSTRAINT nodes; size metric is ``rfc8785_bytes``.

Supported file shape (YAML subset or JSON)::

    default_budget: 50000          # optional; callers may still pass budget=
    always_include_kinds:
      - CONSTRAINT
    size_metric: rfc8785_bytes     # must match the only supported metric today

Unknown top level keys fail closed. Nested expression languages are out of
scope (see issue #75).
"""

from __future__ import annotations

import json
import os
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from trajectory_ir.runtime.projector import SIZE_METRIC

_ENV_POLICY = "TRAJIR_PROJECTOR_POLICY"

_ALLOWED_KEYS = frozenset({"default_budget", "always_include_kinds", "size_metric"})


class PolicyError(ValueError):
    """Raised when a policy file is missing required shape or has unknown keys."""


@dataclass(frozen=True)
class ProjectorPolicy:
    """Resolved projector policy used by :func:`project_context`."""

    always_include_kinds: frozenset[str] = frozenset({"CONSTRAINT"})
    default_budget: int | None = None
    size_metric: str = SIZE_METRIC

    def __post_init__(self) -> None:
        if self.size_metric != SIZE_METRIC:
            raise PolicyError(
                f"unsupported size_metric {self.size_metric!r}; only {SIZE_METRIC!r} is implemented"
            )
        if self.default_budget is not None and self.default_budget < 0:
            raise PolicyError("default_budget must be non-negative")
        if not self.always_include_kinds:
            raise PolicyError("always_include_kinds must be non-empty")
        if "CONSTRAINT" not in self.always_include_kinds:
            # CONSTRAINT must always survive projection (README §4); a
            # policy that omits it is a footgun, not a real customization.
            object.__setattr__(
                self, "always_include_kinds", self.always_include_kinds | {"CONSTRAINT"}
            )


def default_projector_policy() -> ProjectorPolicy:
    """Built in policy (same as pre file behavior)."""
    return ProjectorPolicy()


def policy_from_mapping(data: dict[str, Any]) -> ProjectorPolicy:
    """Build a policy from a plain dict (JSON/YAML load result)."""
    if not isinstance(data, dict):
        raise PolicyError("policy root must be a mapping")
    unknown = set(data) - _ALLOWED_KEYS
    if unknown:
        raise PolicyError(f"unknown policy keys: {sorted(unknown)}")

    kinds = data.get("always_include_kinds", ["CONSTRAINT"])
    if not isinstance(kinds, list) or not all(isinstance(k, str) and k for k in kinds):
        raise PolicyError("always_include_kinds must be a list of non-empty strings")

    budget = data.get("default_budget")
    if budget is not None and not isinstance(budget, int):
        raise PolicyError("default_budget must be an integer when set")

    metric = data.get("size_metric", SIZE_METRIC)
    if not isinstance(metric, str):
        raise PolicyError("size_metric must be a string")

    return ProjectorPolicy(
        always_include_kinds=frozenset(kinds),
        default_budget=budget,
        size_metric=metric,
    )


def _parse_scalar(raw: str) -> Any:
    s = raw.strip()
    if not s or s in {"null", "Null", "NULL", "~"}:
        return None
    if (s.startswith('"') and s.endswith('"')) or (s.startswith("'") and s.endswith("'")):
        return s[1:-1]
    if re.fullmatch(r"-?\d+", s):
        return int(s)
    if s in {"true", "True", "false", "False"}:
        return s.lower() == "true"
    return s


def _load_simple_yaml(text: str) -> dict[str, Any]:
    """Parse the tiny YAML subset we document (mappings + string lists only)."""
    data: dict[str, Any] = {}
    lines = text.splitlines()
    i = 0
    while i < len(lines):
        line = lines[i]
        stripped = line.strip()
        i += 1
        if not stripped or stripped.startswith("#"):
            continue
        if stripped.startswith("-"):
            raise PolicyError("list item outside a key is not supported")
        if ":" not in stripped:
            raise PolicyError(f"expected key: value, got {stripped!r}")
        key, _, rest = stripped.partition(":")
        key = key.strip()
        rest = rest.strip()
        if rest:
            data[key] = _parse_scalar(rest)
            continue
        # block list
        items: list[Any] = []
        while i < len(lines):
            nxt = lines[i]
            ns = nxt.strip()
            if not ns or ns.startswith("#"):
                i += 1
                continue
            if not nxt.startswith((" ", "\t")):
                break
            if not ns.startswith("-"):
                raise PolicyError(f"expected list item under {key!r}, got {ns!r}")
            items.append(_parse_scalar(ns[1:].strip()))
            i += 1
        data[key] = items
    return data


def load_projector_policy(path: str | Path) -> ProjectorPolicy:
    """Load policy from a JSON or YAML subset file."""
    p = Path(path)
    if not p.is_file():
        raise PolicyError(f"policy file not found: {p}")
    text = p.read_text(encoding="utf-8")
    suffix = p.suffix.lower()
    if suffix == ".json":
        try:
            raw = json.loads(text)
        except json.JSONDecodeError as exc:
            raise PolicyError(f"invalid JSON policy: {exc}") from exc
    else:
        # .yaml / .yml / unknown: try JSON first, then simple YAML subset
        try:
            raw = json.loads(text)
        except json.JSONDecodeError:
            raw = _load_simple_yaml(text)
    if not isinstance(raw, dict):
        raise PolicyError("policy root must be a mapping")
    return policy_from_mapping(raw)


def load_projector_policy_from_env(
    env_var: str = _ENV_POLICY,
) -> ProjectorPolicy | None:
    """Load policy from ``TRAJIR_PROJECTOR_POLICY`` path if set; else None."""
    path = os.environ.get(env_var)
    if not path:
        return None
    return load_projector_policy(path)
