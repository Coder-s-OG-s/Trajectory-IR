"""Three-line ADK wrap. The agent stays the agent.

    from integrations.adk import AdkToolGuard

    guard = AdkToolGuard(tenant_id="demo", trajectory_id="adk-live", work_dir=".local-evidence/adk-live")
    agent = Agent(tools=my_tools, before_tool_callback=guard.before_tool_callback, after_tool_callback=guard.after_tool_callback)

``before_tool_callback`` seals and returns None so ADK runs the tool.
``after_tool_callback`` stores the response ADK already has. Neither callback
calls the tool.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

from integrations.hosthook.session import record_tool_result, seal_before_tool


def _tool_name(tool: Any) -> str:
    if isinstance(tool, dict):
        return str(tool.get("name") or "")
    return str(getattr(tool, "name", "") or "")


def _use_id(tool: Any, args: Any, tool_context: Any) -> str:
    for source in (tool_context, tool):
        if source is None:
            continue
        for attr in ("function_call_id", "tool_call_id", "id"):
            value = source.get(attr) if isinstance(source, dict) else getattr(source, attr, None)
            if value:
                return str(value)
    return _tool_name(tool) or "adk-tool"


def _args(args: Any) -> dict[str, Any]:
    if isinstance(args, dict):
        return args
    return {"value": args}


class AdkToolGuard:
    def __init__(self, tenant_id: str, trajectory_id: str, work_dir: str | Path) -> None:
        self.tenant_id = tenant_id
        self.trajectory_id = trajectory_id
        self.work_dir = work_dir

    def before_tool_callback(self, tool: Any, args: Any, tool_context: Any = None) -> None:
        seal_before_tool(
            self.work_dir,
            self.tenant_id,
            self.trajectory_id,
            _tool_name(tool),
            _args(args),
            _use_id(tool, args, tool_context),
        )
        return None

    def after_tool_callback(
        self,
        tool: Any,
        args: Any,
        tool_context: Any = None,
        tool_response: Any = None,
    ) -> None:
        if tool_response is None and tool_context is not None:
            tool_response = getattr(tool_context, "tool_response", None)
        record_tool_result(
            self.work_dir,
            self.tenant_id,
            self.trajectory_id,
            _use_id(tool, args, tool_context),
            tool_response,
        )
        return None
