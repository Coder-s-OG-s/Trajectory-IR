import json
from collections.abc import Mapping
from dataclasses import dataclass, field
from types import SimpleNamespace
from typing import Any

from drivers.durable_backend.dbos.adapter import init_backend
from trajectory_ir.console_emit import (
    OBSERVATION_BUDGET,
    note_node,
    note_projection,
    note_seal,
)
from trajectory_ir.effects import assert_open_world_effect, requires_block_and_gate
from trajectory_ir.resume.gate import make_gated_tool_call, make_plain_tool_call
from trajectory_ir.resume.world import WorldDrift, decision_payload
from trajectory_ir.resume.world import check_world as _check_world
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import payload_hash
from trajectory_ir.runtime.sandbox import RunMode, assert_tool_allowed_in_mode, normalize_run_mode
from trajectory_ir.runtime.tool import Tool

__all__ = [
    "Decision",
    "ProjectContext",
    "ToolResult",
    "Trajectory",
    "WorldDrift",
    "check_world",
    "commit_step",
    "exec_tool",
    "open_trajectory",
    "project",
    "resume",
    "seal_decision",
]


@dataclass
class Trajectory:
    trajectory_id: str
    tenant_id: str
    db_path: str
    mode: RunMode = RunMode.LIVE
    console_sink: Any = None
    _log: NodeLog = field(init=False, repr=False, compare=False)
    _closed: bool = field(init=False, repr=False, compare=False, default=False)

    def __post_init__(self):
        self._log = NodeLog(self.db_path)
        self._closed = False

    def close(self) -> None:
        """Close the underlying NodeLog SQLite connection idempotently."""
        if not self._closed:
            self._closed = True
            self._log.close()

    def __enter__(self) -> "Trajectory":
        return self

    def __exit__(self, exc_type: Any, exc_val: Any, exc_tb: Any) -> None:
        self.close()


@dataclass
class ProjectContext:
    step_n: int
    context: dict


@dataclass
class Decision:
    step_n: int
    plan: dict
    world_snapshot: dict[str, str] | None = None


@dataclass
class ToolResult:
    step_n: int
    result: Any


def open_trajectory(
    tenant_id: str,
    trajectory_id: str,
    db_path: str = "trajectory.sqlite",
    *,
    mode: RunMode | str = RunMode.LIVE,
    console_sink: Any = None,
) -> Trajectory:
    """Open a trajectory.

    ``mode="sandbox"`` rejects NON_IDEMPOTENT_WRITE, AGENT_SPAWN,
    and SENSITIVE tools (R06).
    """
    init_backend(db_path=db_path)
    return Trajectory(
        trajectory_id=trajectory_id,
        tenant_id=tenant_id,
        db_path=db_path,
        mode=normalize_run_mode(mode),
        console_sink=console_sink,
    )


def project(trajectory: Trajectory, step_n: int, context: dict) -> ProjectContext:
    """Record a PROJECT_CONTEXT node (caller-supplied or projector-built context).

    For budgeted assembly from the node log, use
    ``trajectory_ir.runtime.project_context`` first, then pass its
    ``ProjectResult.context`` here.
    """
    node = trajectory._log.append(
        "PROJECT_CONTEXT",
        step_n,
        context,
        trajectory.trajectory_id,
        trajectory.tenant_id,
        seq=0,
    )
    note_node(
        trajectory.console_sink,
        trajectory_id=trajectory.trajectory_id,
        tenant_id=trajectory.tenant_id,
        node=node,
    )
    _emit_projection(trajectory, step_n)
    return ProjectContext(step_n=step_n, context=context)


def seal_decision(
    trajectory: Trajectory,
    step_n: int,
    plan: dict,
    world_snapshot: Mapping[str, str] | None = None,
) -> Decision:
    snap = dict(world_snapshot) if world_snapshot else None
    node = trajectory._log.append(
        "DECISION",
        step_n,
        decision_payload(plan, snap),
        trajectory.trajectory_id,
        trajectory.tenant_id,
        seq=1,
    )
    note_node(
        trajectory.console_sink,
        trajectory_id=trajectory.trajectory_id,
        tenant_id=trajectory.tenant_id,
        node=node,
    )
    note_seal(
        trajectory.console_sink,
        trajectory_id=trajectory.trajectory_id,
        tenant_id=trajectory.tenant_id,
        node=node,
        step_n=step_n,
        plan=plan,
    )
    return Decision(step_n=step_n, plan=plan, world_snapshot=snap)


def check_world(
    trajectory: Trajectory,
    step_n: int,
    observed: Mapping[str, str],
) -> None:
    """Fail-loud WORLD_DRIFT if a sealed snapshot no longer matches.

    No-op when the DECISION has no world_snapshot. Raises ValueError if
    this step has no DECISION yet.
    """
    _check_world(
        trajectory._log,
        trajectory.trajectory_id,
        trajectory.tenant_id,
        step_n,
        observed,
    )


def exec_tool(trajectory: Trajectory, step_n: int, call: dict, tool: Tool, seq: int) -> ToolResult:
    """Execute a tool call within a step.

    Args:
        trajectory: The trajectory context
        step_n: Step number
        call: Tool call dict with "args" key
        tool: Tool definition with name, fn, and effect_class
        seq: Sequence number within the step (caller-supplied, must be unique per call)
             Suggested allocation: seq = 2 + 2*i for i-th tool call in a step

    Returns:
        ToolResult with the tool execution result
    """
    assert_open_world_effect(
        tool.name,
        tool.effect_class,
        allow_override=tool.allow_open_world_override,
    )
    assert_tool_allowed_in_mode(
        trajectory.mode,
        tool_name=tool.name,
        effect_class=tool.effect_class,
    )
    log = trajectory._log
    try:
        if requires_block_and_gate(tool.effect_class):
            fn = make_gated_tool_call(
                log,
                trajectory.trajectory_id,
                trajectory.tenant_id,
                step_n,
                seq=seq,
                tool_name=tool.name,
                tool_fn=tool.fn,
            )
            result = fn(**call["args"])
        else:
            fn = make_plain_tool_call(
                log,
                trajectory.trajectory_id,
                trajectory.tenant_id,
                step_n,
                seq=seq,
                tool_name=tool.name,
                tool_fn=tool.fn,
            )
            result = fn(**call["args"])
    except Exception:
        _emit_tool_console(trajectory, step_n, seq, tool)
        raise
    _emit_tool_console(trajectory, step_n, seq, tool)
    return ToolResult(step_n=step_n, result=result)


def _emit_tool_console(trajectory: Trajectory, step_n: int, seq: int, tool: Tool) -> None:
    """Emit node.appended for TOOL_CALL / outcome rows after exec_tool."""
    if trajectory.console_sink is None:
        return
    try:
        rows = trajectory._log.list_nodes(trajectory.trajectory_id, tenant_id=trajectory.tenant_id)
    except Exception:
        return
    effect = getattr(tool.effect_class, "value", None) or str(tool.effect_class)
    for row in rows:
        if row.get("step_n") != step_n:
            continue
        if row.get("seq") not in (seq, seq + 1):
            continue
        payload = row.get("payload")
        body: dict[str, Any] = payload if isinstance(payload, dict) else {}
        try:
            phash = payload_hash(body)
        except Exception:
            phash = ""
        node = SimpleNamespace(
            id=row["id"],
            kind=row["kind"],
            seq=row["seq"],
            step_n=row["step_n"],
            payload=body,
            phash=phash,
        )
        note_node(
            trajectory.console_sink,
            trajectory_id=trajectory.trajectory_id,
            tenant_id=trajectory.tenant_id,
            node=node,
            effect_class=effect if row["kind"] == "TOOL_CALL" else None,
        )


def commit_step(trajectory: Trajectory, step_n: int, seq: int) -> None:
    """Commit (finalize) a step in the trajectory.

    Args:
        trajectory: The trajectory context
        step_n: Step number
        seq: Sequence number for commit (should be 2 + 2*num_tool_calls to follow
            after all tool calls)
    """
    node = trajectory._log.append(
        "COMMIT_STEP",
        step_n,
        {},
        trajectory.trajectory_id,
        trajectory.tenant_id,
        seq=seq,
    )
    note_node(
        trajectory.console_sink,
        trajectory_id=trajectory.trajectory_id,
        tenant_id=trajectory.tenant_id,
        node=node,
    )


def _char_len(rows: list[dict[str, Any]]) -> int:
    items = [{"kind": n.get("kind"), "payload": n.get("payload")} for n in rows]
    return len(json.dumps(items, separators=(",", ":")))


def _emit_projection(trajectory: Trajectory, step_n: int) -> None:
    if trajectory.console_sink is None:
        return
    try:
        from trajectory_ir.runtime.projector import node_size_units, project_context

        rows = trajectory._log.list_nodes(trajectory.trajectory_id, tenant_id=trajectory.tenant_id)
        if not rows:
            return
        raw_units = sum(node_size_units(n) for n in rows)
        result = project_context(rows, budget=OBSERVATION_BUDGET)
        included_ids = list(result.included_ids)
        want = set(included_ids)
        included = [n for n in rows if n.get("id") in want]
        note_projection(
            trajectory.console_sink,
            trajectory_id=trajectory.trajectory_id,
            tenant_id=trajectory.tenant_id,
            step_n=step_n,
            budget=result.budget,
            metric=result.metric,
            size_units=result.size_units,
            raw_size_units=raw_units,
            included_ids=included_ids,
            dropped_ids=list(result.dropped_ids),
            raw_char_len=_char_len(rows),
            projected_char_len=_char_len(included),
        )
    except Exception:
        return


def resume(
    trajectory_id: str,
    tenant_id: str = "demo",
    db_path: str = "trajectory.sqlite",
    *,
    mode: RunMode | str = RunMode.LIVE,
) -> Trajectory:
    """Reattach to a trajectory that already has node log history.

    This SDK does not wrap steps in a durable workflow decorator the way
    ``trajectory_ir.resume.step.make_run_step`` does; crash safety here comes
    entirely from ``NodeLog`` being idempotent by content and from
    ``exec_tool``'s block-and-gate claim on NON_IDEMPOTENT_WRITE tools
    (README S8, R02). Re-driving the same step_n/seq calls against the same
    db_path is what "resume" means at this layer, so there is no separate
    replay step to run here beyond reconnecting to the log.

    Unlike ``open_trajectory``, this raises if ``trajectory_id`` has no prior
    nodes: resuming a trajectory with nothing to resume is almost always a
    caller bug (wrong db_path or trajectory_id), and should fail loudly
    rather than silently behave like ``open_trajectory``.
    """
    init_backend(db_path=db_path)
    traj = Trajectory(
        trajectory_id=trajectory_id,
        tenant_id=tenant_id,
        db_path=db_path,
        mode=normalize_run_mode(mode),
    )
    try:
        nodes = traj._log.list_nodes(trajectory_id, tenant_id=tenant_id)
    except Exception:
        traj.close()
        raise

    if not nodes:
        traj.close()
        raise ValueError(
            f"cannot resume trajectory_id={trajectory_id!r}: no existing nodes "
            f"found in {db_path!r}; use open_trajectory() to start a new one"
        )
    return traj
