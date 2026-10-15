"""Google ADK callbacks that seal before the tool and record after it.

ADK itself is not imported. Pass the callbacks into the agent you already run.
"""

from integrations.adk.guard import AdkToolGuard

__all__ = ["AdkToolGuard"]
