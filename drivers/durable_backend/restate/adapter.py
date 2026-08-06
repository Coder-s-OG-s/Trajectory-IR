"""Restate adapter public surface (same names as the DBOS adapter).

Default implementation for this package is the process local memo backend
(:mod:`drivers.durable_backend.restate.local_memo`). That keeps unit tests and
contributors free of a Restate server while still exercising the injectable
durable hooks on ``make_run_step``.

When you run against a real Restate cluster, replace these wrappers with the
Restate SDK bindings documented in ``drivers/durable_backend/restate/README.md``.
Do not reimplement crash detection, retry, or lease logic inside
``pkg/trajectory_ir`` (master README rule 2).
"""

from __future__ import annotations

from drivers.durable_backend.restate.local_memo import (
    clear_memo,
    durable_infer,
    durable_tool,
    durable_workflow,
    get_workflow_id,
    init_backend,
    set_workflow_id,
    workflow_scope,
)

__all__ = [
    "clear_memo",
    "durable_infer",
    "durable_tool",
    "durable_workflow",
    "get_workflow_id",
    "init_backend",
    "set_workflow_id",
    "workflow_scope",
]
