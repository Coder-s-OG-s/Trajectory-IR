"""Default context projector (README §9 policy note / R04).

Metric: ``rfc8785_bytes`` — byte length of the RFC 8785 (JCS) canonical form of
``{"kind": ..., "payload": ...}`` for each included node. Same canonicalization
as node identity hashing (``rfc8785`` / Go ``jcs``), so Python and Go agree on
budget thresholds. This is deliberately not a model-specific tokenizer.

Default policy (no file, or always_include_kinds: [CONSTRAINT]):
- Always include every always_include_kinds node (default: CONSTRAINT)
- Always include explicitly pinned node ids
- If must-include set alone exceeds budget → ``BudgetImpossible`` (never
  silent drop of a must-include or pinned node)
- Other kinds fill remaining budget in deterministic order
  (step_n ascending, null last; then seq; then id)

Optional file/env policy: see ``trajectory_ir.runtime.policy``.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

try:
    import rfc8785
except ImportError:
    rfc8785 = None  # type: ignore[assignment]

SIZE_METRIC = "rfc8785_bytes"


class BudgetImpossible(Exception):
    """Raised when CONSTRAINT/pinned nodes alone exceed the projection budget (R04)."""

    def __init__(self, message: str, *, required_size: int, budget: int):
        self.required_size = required_size
        self.budget = budget
        super().__init__(message)


@dataclass(frozen=True)
class ProjectResult:
    """Outcome of default projection."""

    included_ids: tuple[str, ...]
    dropped_ids: tuple[str, ...]
    context: dict[str, Any]
    size_units: int
    budget: int
    metric: str = SIZE_METRIC


def node_size_units(node: dict[str, Any]) -> int:
    """Size of one node under the rfc8785_bytes metric (JCS canonical length)."""
    kind = node.get("kind", "")
    payload = node.get("payload")
    if payload is None:
        payload = {}
    if not isinstance(payload, dict):
        raise TypeError("payload must be an object for size measurement")
    if rfc8785 is None:
        raise RuntimeError("rfc8785 is required for size measurement but is not installed")
    # RFC 8785 JCS — same stack as runtime.nodes.payload_hash (byte-stable vs Go jcs).
    canon = rfc8785.dumps({"kind": kind, "payload": payload})
    return len(canon)


def _sort_key(node: dict[str, Any]) -> tuple[int, int, str]:
    step = node.get("step_n")
    step_ord = step if isinstance(step, int) else 10**9
    seq = node.get("seq")
    seq_ord = int(seq) if seq is not None else 10**9
    nid = str(node.get("id") or "")
    return (step_ord, seq_ord, nid)


def project_context(
    nodes: list[dict[str, Any]],
    *,
    budget: int | None = None,
    pinned_ids: set[str] | frozenset[str] | None = None,
    policy: Any | None = None,
) -> ProjectResult:
    """Project nodes into a budgeted context under the default (or file) policy.

    Args:
        nodes: Node records (must include ``id``, ``kind``, ``payload``; optionally
            ``step_n``, ``seq`` for ordering).
        budget: Maximum total size units. When None, uses ``policy.default_budget``
            if set, otherwise raises ValueError.
        pinned_ids: Node ids that must always appear when present in ``nodes``.
        policy: Optional ``ProjectorPolicy``. When None, built in defaults apply
            (always include CONSTRAINT).

    Returns:
        ProjectResult with included/dropped ids and an assembled context payload
        suitable for recording as PROJECT_CONTEXT.

    Raises:
        BudgetImpossible: if always_include + pinned nodes alone exceed ``budget``.
        ValueError: if ``budget`` is negative or cannot be resolved.
    """
    if policy is None:
        from trajectory_ir.runtime.policy import default_projector_policy

        policy = default_projector_policy()
    if budget is None:
        budget = policy.default_budget
    if budget is None:
        raise ValueError("budget is required when policy.default_budget is unset")
    if budget < 0:
        raise ValueError("budget must be non-negative")

    always_kinds = policy.always_include_kinds
    pinned = set(pinned_ids or ())

    must: list[dict[str, Any]] = []
    seen: set[str] = set()
    for n in nodes:
        nid = str(n.get("id") or "")
        if (n.get("kind") in always_kinds or nid in pinned) and (nid and nid not in seen):
            must.append(n)
            seen.add(nid)

    must_sorted = sorted(must, key=_sort_key)
    must_size = sum(node_size_units(n) for n in must_sorted)
    if must_size > budget:
        raise BudgetImpossible(
            f"BUDGET_IMPOSSIBLE: always-include/pinned size {must_size} exceeds budget {budget}",
            required_size=must_size,
            budget=budget,
        )

    included: list[dict[str, Any]] = list(must_sorted)
    included_ids: set[str] = {str(n["id"]) for n in included}
    used = must_size

    optional = sorted(
        (n for n in nodes if str(n.get("id") or "") not in included_ids),
        key=_sort_key,
    )
    for n in optional:
        cost = node_size_units(n)
        if used + cost > budget:
            continue
        included.append(n)
        included_ids.add(str(n["id"]))
        used += cost

    # Keep output order deterministic: must-include order then optional fill order.
    # Re-sort full set for stable packaging.
    included = sorted(included, key=_sort_key)
    dropped = tuple(
        sorted(
            str(n["id"])
            for n in nodes
            if n.get("id") is not None and str(n["id"]) not in included_ids
        )
    )
    items = [
        {
            "id": n["id"],
            "kind": n["kind"],
            "payload": n.get("payload") or {},
            "step_n": n.get("step_n"),
            "seq": n.get("seq"),
        }
        for n in included
    ]
    context: dict[str, Any] = {
        "items": items,
        "included_ids": [str(n["id"]) for n in included],
        "budget": budget,
        "size_units": used,
        "metric": SIZE_METRIC,
    }
    return ProjectResult(
        included_ids=tuple(str(n["id"]) for n in included),
        dropped_ids=dropped,
        context=context,
        size_units=used,
        budget=budget,
        metric=SIZE_METRIC,
    )
