"""Seal-derived idempotency keys (spec §7.3).

Format: ``trajectory_id:step_n:stable_call_id`` where ``stable_call_id`` is
the sealed tool slot ``seq`` (typically ``2 + 2*i``), never a fresh id minted
after a crash.

This value is recorded on the ``TOOL_CALL`` node so a host can forward it to a
remote API as ``Idempotency-Key`` / vendor equivalent. The IR log is not the
server. Client-side block-and-gate is at-most-one automatic attempt from this
process; exactly-once still requires the *callee* to honor the key.
"""


def idempotency_key(trajectory_id: str, step_n: int, seq: int) -> str:
    """Return the spec §7.3 key for a sealed tool slot."""
    return f"{trajectory_id}:{step_n}:{seq}"
