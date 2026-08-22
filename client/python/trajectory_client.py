from dataclasses import dataclass
from typing import Any

from drivers.durable_backend.dbos.adapter import init_backend
from trajectory_ir.effects import requires_block_and_gate
from trajectory_ir.resume.gate import make_gated_tool_call, make_plain_tool_call
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.sandbox import RunMode, assert_tool_allowed_in_mode, normalize_run_mode
from trajectory_ir.runtime.tool import Tool


@dataclass
class Trajectory:
    trajectory_id: str
    tenant_id: str
    db_path: str
    mode: RunMode = RunMode.LIVE


@dataclass
class ProjectContext:
    step_n: int
    context: dict


@dataclass
class Decision:
    step_n: int
    plan: dict


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
) -> Trajectory:
    """Open a trajectory. ``mode=\"sandbox\"`` rejects NON_IDEMPOTENT_WRITE tools (R06)."""
    init_backend(app_name=trajectory_id)
    return Trajectory(
        trajectory_id=trajectory_id,
        tenant_id=tenant_id,
        db_path=db_path,
        mode=normalize_run_mode(mode),
    )


def project(trajectory: Trajectory, step_n: int, context: dict) -> ProjectContext:
    """Record a PROJECT_CONTEXT node (caller-supplied or projector-built context).

    For budgeted assembly from the node log, use
    ``trajectory_ir.runtime.project_context`` first, then pass its
    ``ProjectResult.context`` here.
    """
    NodeLog(trajectory.db_path).append(
        "PROJECT_CONTEXT",
        step_n,
        context,
        trajectory.trajectory_id,
        trajectory.tenant_id,
        seq=0,
    )
    return ProjectContext(step_n=step_n, context=context)


def seal_decision(trajectory: Trajectory, step_n: int, plan: dict) -> Decision:
    NodeLog(trajectory.db_path).append(
        "DECISION",
        step_n,
        {"plan": plan},
        trajectory.trajectory_id,
        trajectory.tenant_id,
        seq=1,
    )
    return Decision(step_n=step_n, plan=plan)


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
    assert_tool_allowed_in_mode(
        trajectory.mode,
        tool_name=tool.name,
        effect_class=tool.effect_class,
    )
    log = NodeLog(trajectory.db_path)
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
    return ToolResult(step_n=step_n, result=result)


def commit_step(trajectory: Trajectory, step_n: int, seq: int) -> None:
    """Commit (finalize) a step in the trajectory.

    Args:
        trajectory: The trajectory context
        step_n: Step number
        seq: Sequence number for commit (should be 2 + 2*num_tool_calls to follow
            after all tool calls)
    """
    NodeLog(trajectory.db_path).append(
        "COMMIT_STEP",
        step_n,
        {},
        trajectory.trajectory_id,
        trajectory.tenant_id,
        seq=seq,
    )


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
    init_backend(app_name=trajectory_id)
    if not NodeLog(db_path).list_nodes_all_tenants(trajectory_id):
        raise ValueError(
            f"cannot resume trajectory_id={trajectory_id!r}: no existing nodes "
            f"found in {db_path!r}; use open_trajectory() to start a new one"
        )
    return Trajectory(
        trajectory_id=trajectory_id,
        tenant_id=tenant_id,
        db_path=db_path,
        mode=normalize_run_mode(mode),
    )
