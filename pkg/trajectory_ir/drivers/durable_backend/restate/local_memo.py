"""Process local durable step memoization (dev / test stand in for Restate).

Mirrors the DBOS adapter contract used by ``make_run_step``:

* ``durable_workflow`` marks the host entrypoint (no special behavior here)
* ``durable_infer`` / ``durable_tool`` run the body once per
  (workflow id, step name, args) and return the memoized result on replay

Arguments have to be JSON. The process keeps at most 10000 results and drops
the least recently used. A call after that drop runs the body again.

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
from collections import OrderedDict
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from contextvars import ContextVar
from typing import Any, TypeVar

F = TypeVar("F", bound=Callable[..., Any])

_workflow_id: ContextVar[str] = ContextVar("restate_local_workflow_id", default="")
_step_seq: ContextVar[int] = ContextVar("restate_local_step_seq", default=0)
_lock = threading.RLock()
_MEMO_LIMIT = 10000
_MISSING = object()
_ARGS_ERR = "restate local_memo: step arguments must be JSON-serializable"
# key: (workflow_id, seq, step_kind, step_name, args_json) -> result
_memo: OrderedDict[tuple[str, int, str, str, str], Any] = OrderedDict()


def set_workflow_id(workflow_id: str) -> None:
    """Bind the current context to a durable workflow id."""
    if not workflow_id:
        raise ValueError("workflow_id is required")
    _workflow_id.set(workflow_id)
    _step_seq.set(0)


def get_workflow_id() -> str:
    return _workflow_id.get()


@contextmanager
def workflow_scope(workflow_id: str) -> Iterator[None]:
    """Context manager equivalent of DBOS ``SetWorkflowID``."""
    token = _workflow_id.set(workflow_id)
    seq_token = _step_seq.set(0)
    try:
        yield
    finally:
        _workflow_id.reset(token)
        _step_seq.reset(seq_token)


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


def _json_value(value: Any, seen: set[int] | None = None) -> Any:
    if value is None or isinstance(value, (str, int, float)):
        return value
    if seen is None:
        seen = set()
    if isinstance(value, list):
        marker = id(value)
        if marker in seen:
            raise ValueError(_ARGS_ERR)
        seen.add(marker)
        try:
            return [_json_value(item, seen) for item in value]
        finally:
            seen.discard(marker)
    if isinstance(value, dict):
        marker = id(value)
        if marker in seen:
            raise ValueError(_ARGS_ERR)
        seen.add(marker)
        try:
            encoded: dict[str, Any] = {}
            for key, item in value.items():
                if not isinstance(key, str):
                    raise ValueError(_ARGS_ERR)
                encoded[key] = _json_value(item, seen)
            return encoded
        finally:
            seen.discard(marker)
    raise ValueError(_ARGS_ERR)


def _args_key(args: tuple[Any, ...], kwargs: dict[str, Any]) -> str:
    payload = {"args": _json_value(list(args)), "kwargs": _json_value(kwargs)}
    try:
        return json.dumps(payload, sort_keys=True)
    except (TypeError, ValueError) as exc:
        raise ValueError(_ARGS_ERR) from exc


def _cached(key: tuple[str, int, str, str, str]) -> Any:
    with _lock:
        found = _memo.get(key, _MISSING)
        if found is _MISSING:
            return _MISSING
        _memo.move_to_end(key)
        return found


def _save(key: tuple[str, int, str, str, str], result: Any) -> Any:
    with _lock:
        # First writer wins if two threads race the miss path.
        if key not in _memo:
            _memo[key] = result
            limit = _MEMO_LIMIT if _MEMO_LIMIT >= 1 else 1
            while len(_memo) > limit:
                _memo.popitem(last=False)
        else:
            _memo.move_to_end(key)
        return _memo[key]


def _memoize(step_kind: str, fn: F) -> F:
    step_name = getattr(fn, "__qualname__", getattr(fn, "__name__", repr(fn)))

    def wrapper(*args: Any, **kwargs: Any) -> Any:
        wid = get_workflow_id()
        if not wid:
            raise RuntimeError(
                "restate local_memo: workflow id not set; "
                "call set_workflow_id() or use workflow_scope() before durable steps"
            )
        args_key = _args_key(args, kwargs)
        seq = _step_seq.get()
        _step_seq.set(seq + 1)
        key = (wid, seq, step_kind, step_name, args_key)
        cached = _cached(key)
        if cached is not _MISSING:
            return cached
        result = fn(*args, **kwargs)
        return _save(key, result)

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
    """Mark a function as a durable workflow entrypoint (namespaces by step_n locally)."""

    def wrapper(*args: Any, **kwargs: Any) -> Any:
        step_n_val = kwargs.get("step_n")
        if step_n_val is None and args and isinstance(args[0], int):
            step_n_val = args[0]

        old_wid = get_workflow_id()
        if old_wid and step_n_val is not None:
            token = _workflow_id.set(f"{old_wid}-step{step_n_val}")
            seq_token = _step_seq.set(0)
        else:
            token = None
            seq_token = None

        try:
            return fn(*args, **kwargs)
        finally:
            if token is not None:
                _workflow_id.reset(token)
            if seq_token is not None:
                _step_seq.reset(seq_token)

    wrapper.__name__ = getattr(fn, "__name__", "wrapped")
    wrapper.__qualname__ = getattr(fn, "__qualname__", wrapper.__name__)
    return wrapper  # type: ignore[return-value]
