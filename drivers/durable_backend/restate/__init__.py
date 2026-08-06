"""Restate durable backend adapter (optional second implementation).

Phase 1A still defaults to DBOS via ``make_run_step``. This package exposes
the same call surface so a host can inject Restate style durability without
reimplementing crash detection inside Trajectory IR.

Development and unit tests use :mod:`drivers.durable_backend.restate.local_memo`
(process local step memoization). A real Restate cluster is optional and
documented in ``README.md`` in this directory.
"""

from drivers.durable_backend.restate import local_memo as local_memo
from drivers.durable_backend.restate.adapter import (
    durable_infer,
    durable_tool,
    durable_workflow,
    init_backend,
)

__all__ = [
    "durable_infer",
    "durable_tool",
    "durable_workflow",
    "init_backend",
    "local_memo",
]
