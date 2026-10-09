"""Seal-derived idempotency keys (spec §7.3).

The key is ``hex(sha256(length-prefixed fields))`` with domain
``trajir-idempotency-v1``. Each field is ``uint32be(byte length) || utf-8``,
in order: domain, tenant, trajectory, decimal step, decimal seq.

Length prefixes stop a NUL inside a tenant or trajectory id from aliasing
a different pair. The NUL-joined draft collides: ``("a", "b\\x00c")`` and
``("a\\x00b", "c")`` hash the same.

A trajectory sealed under the withdrawn colon key
(``trajectory:step:seq``, no tenant) does not resume onto this hash.
Pre-1.0, there is no translator. Forwarding the new key for an in-flight
call can repeat the side effect at the callee.

The key is recorded on the ``TOOL_CALL`` node and exposed to the tool body
via :class:`CallMeta` / :func:`current_call_meta` so a host can forward it
as ``Idempotency-Key``. It is never injected into the tool's user arguments.

The IR log is not the server. Client-side block-and-gate is at-most-one
automatic attempt from this process; exactly-once still requires the
*callee* to honor the key.
"""

from __future__ import annotations

import contextvars
import hashlib
from collections.abc import Mapping
from dataclasses import dataclass

IDEMPOTENCY_DOMAIN = "trajir-idempotency-v1"


@dataclass(frozen=True)
class CallMeta:
    """Context for one sealed tool slot. Available inside the tool body."""

    tenant_id: str
    trajectory_id: str
    step_n: int
    seq: int
    idempotency_key: str

    @classmethod
    def make(cls, tenant_id: str, trajectory_id: str, step_n: int, seq: int) -> CallMeta:
        return cls(
            tenant_id=tenant_id,
            trajectory_id=trajectory_id,
            step_n=step_n,
            seq=seq,
            idempotency_key=idempotency_key(tenant_id, trajectory_id, step_n, seq),
        )


_current_meta: contextvars.ContextVar[CallMeta | None] = contextvars.ContextVar(
    "trajir_call_meta",
    default=None,
)


def _len_prefixed(field: str) -> bytes:
    raw = field.encode("utf-8")
    return len(raw).to_bytes(4, "big") + raw


def idempotency_key(tenant_id: str, trajectory_id: str, step_n: int, seq: int) -> str:
    """Return the spec §7.3 hashed key for a sealed tool slot.

    Fields are length-prefixed. A trajectory sealed with the withdrawn colon
    key does not resume onto this hash (pre-1.0, no translator).
    """
    raw = b"".join(
        _len_prefixed(field)
        for field in (
            IDEMPOTENCY_DOMAIN,
            tenant_id,
            trajectory_id,
            str(step_n),
            str(seq),
        )
    )
    return hashlib.sha256(raw).hexdigest()


def idempotency_key_header(key: str) -> dict[str, str]:
    """HTTP header map a host can merge onto an outbound request."""
    return {"Idempotency-Key": key}


def current_call_meta() -> CallMeta | None:
    """CallMeta for the tool body currently running, or None outside a call."""
    return _current_meta.get()


def current_idempotency_key() -> str | None:
    """Hashed key for the tool body currently running, or None outside a call."""
    meta = _current_meta.get()
    return None if meta is None else meta.idempotency_key


def bind_call_meta(meta: CallMeta, tool_fn, kwargs: Mapping[str, object]):
    """Run ``tool_fn(**kwargs)`` with :func:`current_call_meta` bound.

    kwargs are passed through unchanged. The key is not added to them.
    """
    token = _current_meta.set(meta)
    try:
        return tool_fn(**kwargs)
    finally:
        _current_meta.reset(token)
