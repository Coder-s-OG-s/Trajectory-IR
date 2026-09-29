"""Optional WORLD_SNAPSHOT on DECISION and fail-loud CheckWorld (spec §8.4).

A host that cares whether the world still matches the plan it sealed may
put a string-to-string snapshot on the DECISION payload. On resume (or
immediately before tools) the host observes again and calls
:func:`check_world`. Divergence is ``WORLD_DRIFT``: do not re-infer, do
not silently continue.

An empty or absent snapshot is a no-op. Trajectory IR does not invent
what "the world" is and does not pause AWS while the worker is dead.
"""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any


class WorldDrift(Exception):
    """Sealed WORLD_SNAPSHOT no longer matches the host's observation."""

    def __init__(self, step_n: int, mismatches: list[dict[str, str]]):
        self.step_n = step_n
        self.mismatches = mismatches
        super().__init__(
            f"WORLD_DRIFT: step {step_n}: {len(mismatches)} sealed snapshot "
            "key(s) diverged"
        )


def normalize_world_snapshot(
    snapshot: Mapping[str, str] | None,
) -> dict[str, str]:
    """Copy a host-declared snapshot. Empty/None becomes {}."""
    if not snapshot:
        return {}
    if not isinstance(snapshot, Mapping):
        raise TypeError("world_snapshot must be a string-to-string map")
    out: dict[str, str] = {}
    for key, value in snapshot.items():
        if not isinstance(key, str) or not isinstance(value, str):
            raise TypeError("world_snapshot keys and values must be strings")
        out[key] = value
    return out


def decision_payload(
    plan: dict,
    world_snapshot: Mapping[str, str] | None = None,
) -> dict[str, Any]:
    """DECISION payload. Omits world_snapshot when empty so hashes stay stable."""
    payload: dict[str, Any] = {"plan": plan}
    snap = normalize_world_snapshot(world_snapshot)
    if snap:
        payload["world_snapshot"] = snap
    return payload


def sealed_world_snapshot(
    node_log,
    trajectory_id: str,
    tenant_id: str,
    step_n: int,
) -> dict[str, str] | None:
    """Return the sealed snapshot, {} if present-but-empty, or None if no DECISION."""
    for row in node_log.list_nodes(trajectory_id, tenant_id=tenant_id):
        if row["kind"] == "DECISION" and row["step_n"] == step_n:
            raw = row["payload"].get("world_snapshot")
            if raw is None:
                return {}
            return normalize_world_snapshot(raw)
    return None


def check_world(
    node_log,
    trajectory_id: str,
    tenant_id: str,
    step_n: int,
    observed: Mapping[str, str],
) -> None:
    """Compare host observation against the sealed WORLD_SNAPSHOT.

    Extra keys in ``observed`` are ignored. Missing or changed sealed keys
    raise :class:`WorldDrift`. No DECISION is a programming error.
    An empty sealed snapshot is a no-op.
    """
    sealed = sealed_world_snapshot(node_log, trajectory_id, tenant_id, step_n)
    if sealed is None:
        raise ValueError(
            f"no DECISION for step {step_n}; cannot check world snapshot"
        )
    if not sealed:
        return
    seen = normalize_world_snapshot(observed)
    mismatches: list[dict[str, str]] = []
    for key, want in sealed.items():
        got = seen.get(key)
        if got != want:
            mismatches.append(
                {
                    "key": key,
                    "sealed": want,
                    "observed": "" if got is None else got,
                }
            )
    if mismatches:
        raise WorldDrift(step_n, mismatches)
