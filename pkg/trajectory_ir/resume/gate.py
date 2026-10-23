import hashlib
import json

from trajectory_ir.resume.idempotency import CallMeta, bind_call_meta
from trajectory_ir.runtime.nodes import UnsafeInteger, reject_unsafe_integers

_INTEGER_DOMAIN = "integer_domain"


class BlockedNeedsGate(Exception):
    """Raised when a NON_IDEMPOTENT_WRITE tool call was interrupted mid-
    execution and must not be silently retried.

    Raising is the gate. Resolution is a host/human decision (spec §8.3):
    confirm the remote write succeeded, confirm it never landed, compensate,
    or re-run in a *new* step. This client cannot know which of those is
    true (Two Generals). Do not auto-retry and do not re-infer.
    """

    def __init__(self, step_n: int, tool_name: str):
        self.step_n = step_n
        self.tool_name = tool_name
        super().__init__(
            f"step {step_n}: '{tool_name}' BLOCKED_NEEDS_GATE (crashed mid-execution, not retried)"
        )

    def __reduce__(self):
        # The durable backend replays a crashed workflow on a recovery thread and
        # hands the failure back to the caller through its system database, so
        # this exception must survive a serialization round trip. The default
        # Exception reduce would replay only `args` (the message) into a
        # two-argument __init__ and raise TypeError instead of the block.
        return (self.__class__, (self.step_n, self.tool_name))


def _result_digest(result) -> str:
    try:
        text = json.dumps(result, sort_keys=True, separators=(",", ":"), default=str)
    except Exception:
        try:
            text = repr(result)
        except Exception:
            text = type(result).__name__
    return hashlib.sha256(text.encode()).hexdigest()


def _integer_failure_payload(message: str, result) -> dict:
    return {
        "error": _INTEGER_DOMAIN,
        "message": message,
        "result_digest": _result_digest(result),
    }


def _stored_integer_failure(node_log, trajectory_id, tenant_id, step_n, seq):
    if not node_log.has(trajectory_id, tenant_id, step_n, "TOOL_RESULT", seq=seq):
        return None
    for row in node_log.list_nodes(trajectory_id, tenant_id=tenant_id):
        if row["step_n"] != step_n or row["seq"] != seq or row["kind"] != "TOOL_RESULT":
            continue
        payload = row["payload"]
        if (
            isinstance(payload, dict)
            and payload.get("error") == _INTEGER_DOMAIN
            and isinstance(payload.get("result_digest"), str)
        ):
            return payload
    return None


def _raise_stored_integer_failure(payload: dict) -> None:
    message = payload.get("message")
    if not isinstance(message, str) or message == "":
        message = "integer exceeds safe integer domain for JSON floats"
    raise UnsafeInteger(message)


def _append_tool_result(node_log, trajectory_id, tenant_id, step_n, seq, result) -> None:
    payload = {"result": result}
    try:
        reject_unsafe_integers(payload)
    except UnsafeInteger as exc:
        node_log.append(
            "TOOL_RESULT",
            step_n,
            _integer_failure_payload(str(exc), result),
            trajectory_id,
            tenant_id,
            seq,
        )
        raise
    node_log.append(
        "TOOL_RESULT",
        step_n,
        payload,
        trajectory_id,
        tenant_id,
        seq,
    )


def make_gated_tool_call(node_log, trajectory_id, tenant_id, step_n, seq, tool_name, tool_fn):
    """Wrap a NON_IDEMPOTENT_WRITE tool so that a crash anywhere after the call
    started blocks instead of silently re-running the side effect on resume.

    Only for effects where ``requires_block_and_gate`` is true (R02). PURE tools
    (R03) must not use this wrapper — they may recompute freely on resume.

    Uses our own content-addressed NodeLog as the source of truth for "was this
    call already attempted," rather than DBOS's internal workflow-status API --
    this avoids depending on an internal API that may change between DBOS
    versions, at the cost of one extra durable log write before the effect runs.

    Occupies two seq slots for this call: TOOL_CALL at `seq`, and its outcome
    (TOOL_RESULT, or ABORT when blocked) at `seq + 1`. Callers must space tool
    calls accordingly so no two nodes in a step collide on seq -- see
    `resume/step.py`.

    The claim of the TOOL_CALL slot is atomic (``BEGIN IMMEDIATE`` + insert)
    so concurrent workers cannot both pass a non-atomic has-then-append race.
    """

    def gated(**kwargs):
        # Atomic claim: first successful insert of TOOL_CALL at this seq wins.
        # Re-entry (or a concurrent loser) must block — never re-run the effect.
        #
        # Deliberately NOT `TOOL_CALL and not TOOL_RESULT`: TOOL_RESULT is
        # appended and committed to SQLite before the durable backend records
        # the step's memoized output, so a crash in that window leaves *both*
        # nodes present. A `not TOOL_RESULT` conjunct fails open there.
        reject_unsafe_integers({"args": kwargs})
        meta = CallMeta.make(tenant_id, trajectory_id, step_n, seq)
        claimed = node_log.claim_tool_call(
            step_n,
            {
                "tool": tool_name,
                "args": kwargs,
                "idempotency_key": meta.idempotency_key,
            },
            trajectory_id,
            tenant_id,
            seq,
        )
        if not claimed:
            failed = _stored_integer_failure(node_log, trajectory_id, tenant_id, step_n, seq + 1)
            if failed is not None:
                _raise_stored_integer_failure(failed)
            node_log.append(
                "ABORT",
                step_n,
                {"reason": "BLOCKED_NEEDS_GATE", "tool": tool_name},
                trajectory_id,
                tenant_id,
                seq + 1,
            )
            raise BlockedNeedsGate(step_n, tool_name)

        result = bind_call_meta(meta, tool_fn, kwargs)
        _append_tool_result(node_log, trajectory_id, tenant_id, step_n, seq + 1, result)
        return result

    return gated


def make_plain_tool_call(node_log, trajectory_id, tenant_id, step_n, seq, tool_name, tool_fn):
    """Wrap a non-gated tool with audit logging.

    Occupies two seq slots: TOOL_CALL at `seq`, and TOOL_RESULT at `seq + 1`.
    TOOL_CALL is recorded prior to tool execution so failed attempts are audited.
    On success, TOOL_RESULT is recorded at seq + 1 with {"result": ...}.
    """

    def plain(**kwargs):
        reject_unsafe_integers({"args": kwargs})
        failed = _stored_integer_failure(node_log, trajectory_id, tenant_id, step_n, seq + 1)
        if failed is not None:
            _raise_stored_integer_failure(failed)
        meta = CallMeta.make(tenant_id, trajectory_id, step_n, seq)
        node_log.append(
            "TOOL_CALL",
            step_n,
            {
                "tool": tool_name,
                "args": kwargs,
                "idempotency_key": meta.idempotency_key,
            },
            trajectory_id,
            tenant_id,
            seq=seq,
        )

        result = bind_call_meta(meta, tool_fn, kwargs)
        _append_tool_result(node_log, trajectory_id, tenant_id, step_n, seq + 1, result)
        return result

    return plain
