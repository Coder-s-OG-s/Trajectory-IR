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
    """Wrap a NON_IDEMPOTENT_WRITE tool so a crash between 'started' and
    'completed' blocks instead of silently re-running the side effect on
    resume.

    Uses our own content-addressed NodeLog as the source of truth for "was
    this call started but never finished," rather than DBOS's internal
    workflow-status API -- this avoids depending on an internal API that may
    change between DBOS versions, at the cost of one extra durable log write
    before the effect runs.
    """

    def gated(**kwargs):
        if node_log.has(trajectory_id, step_n, "TOOL_CALL") and not node_log.has(
            trajectory_id, step_n, "TOOL_RESULT"
        ):
            node_log.append(
                "ABORT", step_n, {"reason": "BLOCKED_NEEDS_GATE", "tool": tool_name},
                trajectory_id, tenant_id, seq,
            )
            raise BlockedNeedsGate(step_n, tool_name)

        node_log.append(
            "TOOL_CALL", step_n, {"tool": tool_name, "args": kwargs},
            trajectory_id, tenant_id, seq,
        )
        result = tool_fn(**kwargs)
        node_log.append(
            "TOOL_RESULT", step_n, {"result": result},
            trajectory_id, tenant_id, seq + 1,
        )
        return result

    return gated
