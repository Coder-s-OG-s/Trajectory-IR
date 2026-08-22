class BlockedNeedsGate(Exception):
    """Raised when a NON_IDEMPOTENT_WRITE tool call was interrupted mid-
    execution and must not be silently retried. Resolving the block (manual
    replay/approval) is out of scope for this milestone -- raising is the
    gate."""

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
        claimed = node_log.claim_tool_call(
            step_n,
            {"tool": tool_name, "args": kwargs},
            trajectory_id,
            tenant_id,
            seq,
        )
        if not claimed:
            node_log.append(
                "ABORT",
                step_n,
                {"reason": "BLOCKED_NEEDS_GATE", "tool": tool_name},
                trajectory_id,
                tenant_id,
                seq + 1,
            )
            raise BlockedNeedsGate(step_n, tool_name)

        result = tool_fn(**kwargs)
        node_log.append(
            "TOOL_RESULT",
            step_n,
            {"result": result},
            trajectory_id,
            tenant_id,
            seq + 1,
        )
        return result

    return gated


def make_plain_tool_call(node_log, trajectory_id, tenant_id, step_n, seq, tool_name, tool_fn):
    """Wrap a non-gated tool with audit logging.

    Occupies two seq slots: TOOL_CALL at `seq`, and TOOL_RESULT at `seq + 1`.
    TOOL_CALL is recorded prior to tool execution so failed attempts are audited.
    On success, TOOL_RESULT is recorded at seq + 1 with {"result": ...}.
    """

    def plain(**kwargs):
        node_log.append(
            "TOOL_CALL",
            step_n,
            {"tool": tool_name, "args": kwargs},
            trajectory_id,
            tenant_id,
            seq=seq,
        )

        result = tool_fn(**kwargs)

        node_log.append(
            "TOOL_RESULT",
            step_n,
            {"result": result},
            trajectory_id,
            tenant_id,
            seq=seq + 1,
        )
        return result

    return plain
