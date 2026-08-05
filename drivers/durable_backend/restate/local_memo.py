"""Process local durable step memoization (dev / test stand in for Restate).

Mirrors the DBOS adapter contract used by ``make_run_step``:

* ``durable_workflow`` marks the host entrypoint (no special behavior here)
* ``durable_infer`` / ``durable_tool`` run the body once per
  (workflow id, step name, args) and return the memoized result on replay

This is **not** a production Restate deployment. It exists so:

1. The Restate adapter package is testable without a Restate server
2. Hosts can prove R01 style "do not re invoke model" semantics with the
   same ``make_run_step(..., durable_*=...)`` injection path

Set the workflow id with :func:`set_workflow_id` (or the context manager
:func:`workflow_scope`) before invoking a workflow, analogous to DBOS
``SetWorkflowID``.
"""

from __future__ import annotations

import json
import threading
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from contextvars import ContextVar
from typing import Any, TypeVar

F = TypeVar("F", bound=Callable[..., Any])

_workflow_id: ContextVar[str] = ContextVar("restate_local_workflow_id", default="")
_lock = threading.RLock()
# key: (workflow_id, step_kind, step_name, args_json) -> result
_memo: dict[tuple[str, str, str, str], Any] = {}


def set_workflow_id(workflow_id: str) -> None:
    """Bind the current context to a durable workflow id."""
    if not workflow_id:
        raise ValueError("workflow_id is required")
    _workflow_id.set(workflow_id)


def get_workflow_id() -> str:
    return _workflow_id.get()


@contextmanager
def workflow_scope(workflow_id: str) -> Iterator[None]:
    """Context manager equivalent of DBOS ``SetWorkflowID``."""
    token = _workflow_id.set(workflow_id)
    try:
        yield
    finally:
        _workflow_id.reset(token)


def clear_memo() -> None:
    """Drop all memoized steps (tests only)."""
    with _lock:
        _memo.clear()


def init_backend(app_name: str = "trajectory-ir-restate-local") -> None:
    """Initialize the local memo backend.

    ``app_name`` is accepted for API parity with the DBOS adapter; the local
    backend does not open network connections.
    """
    del app_name  # API parity; unused for process local storage.
    # Intentionally do not clear memo on every init: resume scenarios need it.


def _args_key(args: tuple[Any, ...], kwargs: dict[str, Any]) -> str:
    payload = {"args": list(args), "kwargs": kwargs}
    return json.dumps(payload, sort_keys=True, default=str)


def _memoize(step_kind: str, fn: F) -> F:
    step_name = getattr(fn, "__qualname__", getattr(fn, "__name__", repr(fn)))

    def wrapper(*args: Any, **kwargs: Any) -> Any:
        wid = get_workflow_id()
        if not wid:
            raise RuntimeError(
                "restate local_memo: workflow id not set; "
                "call set_workflow_id() or use workflow_scope() before durable steps"
            )
        key = (wid, step_kind, step_name, _args_key(args, kwargs))
        with _lock:
            if key in _memo:
                return _memo[key]
        result = fn(*args, **kwargs)
        with _lock:
            # First writer wins if two threads race the miss path.
            if key not in _memo:
                _memo[key] = result
            return _memo[key]

    wrapper.__name__ = getattr(fn, "__name__", "wrapped")
    wrapper.__qualname__ = step_name
    return wrapper  # type: ignore[return-value]


def durable_infer(fn: F) -> F:
    """Wrap model inference as a memoized durable step."""
    return _memoize("infer", fn)


def durable_tool(fn: F) -> F:
    """Wrap a tool body as a memoized durable step."""
    return _memoize("tool", fn)


def durable_workflow(fn: F) -> F:
    """Mark a function as a durable workflow entrypoint (passthrough locally)."""
    return fn
