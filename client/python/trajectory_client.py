from dataclasses import dataclass
from typing import Any

from drivers.durable_backend.dbos.adapter import init_backend
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import make_gated_tool_call
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool


@dataclass
class Trajectory:
    trajectory_id: str
    tenant_id: str
    db_path: str


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


def open_trajectory(tenant_id: str, trajectory_id: str, db_path: str = "trajectory.sqlite") -> Trajectory:
    init_backend(app_name=trajectory_id)
    return Trajectory(trajectory_id=trajectory_id, tenant_id=tenant_id, db_path=db_path)


def project(trajectory: Trajectory, step_n: int, context: dict) -> ProjectContext:
    NodeLog(trajectory.db_path).append(
        "PROJECT_CONTEXT", step_n, context, trajectory.trajectory_id, trajectory.tenant_id, seq=0
    )
    return ProjectContext(step_n=step_n, context=context)


def seal_decision(trajectory: Trajectory, step_n: int, plan: dict) -> Decision:
    NodeLog(trajectory.db_path).append(
        "DECISION", step_n, {"plan": plan}, trajectory.trajectory_id, trajectory.tenant_id, seq=1
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
    log = NodeLog(trajectory.db_path)
    if tool.effect_class == EffectClass.NON_IDEMPOTENT_WRITE:
        fn = make_gated_tool_call(
            log, trajectory.trajectory_id, trajectory.tenant_id, step_n, seq=seq,
            tool_name=tool.name, tool_fn=tool.fn,
        )
    else:
        fn = tool.fn
    result = fn(**call["args"])
    return ToolResult(step_n=step_n, result=result)


def commit_step(trajectory: Trajectory, step_n: int, seq: int) -> None:
    """Commit (finalize) a step in the trajectory.

    Args:
        trajectory: The trajectory context
        step_n: Step number
        seq: Sequence number for commit (should be 2 + 2*num_tool_calls to follow after all tool calls)
    """
    NodeLog(trajectory.db_path).append(
        "COMMIT_STEP", step_n, {}, trajectory.trajectory_id, trajectory.tenant_id, seq=seq
    )


def resume(trajectory_id: str, tenant_id: str = "demo", db_path: str = "trajectory.sqlite") -> Trajectory:
    init_backend(app_name=trajectory_id)
    return Trajectory(trajectory_id=trajectory_id, tenant_id=tenant_id, db_path=db_path)
