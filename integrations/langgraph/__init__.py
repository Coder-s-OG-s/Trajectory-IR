"""LangGraph evidence sidecar (local TURNING_POINT build).

Optional. Install langgraph only when you use ToolNode wrap_tool_call.
Core sealing works through TrajIRToolGuard without LangGraph.
"""

from integrations.langgraph.sidecar import TrajIRToolGuard

__all__ = ["TrajIRToolGuard"]
