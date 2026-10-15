"""Seal a tool call before the host runs it, then record the result after.

Used by the Claude hook and the ADK callbacks. The host still executes the
tool. This package does not call the tool body.
"""

from integrations.hosthook.session import record_tool_result, seal_before_tool

__all__ = ["record_tool_result", "seal_before_tool"]
