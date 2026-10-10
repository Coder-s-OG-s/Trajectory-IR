"""In-process evidence wrap: seal, then classify, then hashed key, then the tool.

The wrap seals before the tool body runs. A post-hoc OTel collector cannot
replace this wrap. Each distinct tool plan is one turn: a later plan commits
the open step and seals the next step.
"""

from __future__ import annotations

import json
import os
import threading
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path
from types import SimpleNamespace
from typing import Any

from client.python.trajectory_client import (
    commit_step,
    exec_tool,
    open_trajectory,
    project,
    seal_decision,
)
from trajectory_ir.console_emit import from_env, note_package
from trajectory_ir.effects import EffectClass, assert_open_world_effect, classify_tool
from trajectory_ir.package import export_tir
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool


def _as_mapping(args: Any) -> dict[str, Any]:
    if args is None:
        return {}
    if isinstance(args, Mapping):
        return dict(args)
    return {"value": args}


def _plan_key_and_calls(
    tool_calls: Sequence[Mapping[str, Any]],
) -> tuple[str, list[dict[str, Any]]]:
    plan_calls: list[dict[str, Any]] = []
    for call in tool_calls:
        plan_calls.append(
            {
                "name": str(call["name"]),
                "args": _as_mapping(call.get("args")),
            }
        )
    key = json.dumps(plan_calls, sort_keys=True, default=str, separators=(",", ":"))
    return key, plan_calls


@dataclass
class TrajIRToolGuard:
    """Seal each model turn into a Trajectory IR NodeLog, then run its tools.

    Use with LangGraph::

        guard = TrajIRToolGuard(tenant_id="demo", trajectory_id="lg-1", work_dir=".")
        ToolNode(tools, wrap_tool_call=guard.wrap_tool_call)

    Or call ``run_sealed`` from tests without LangGraph installed.

    One plan is one turn. Parallel tools that share that plan take a lock so
    each call gets its own sequence number. A different plan commits the open
    step and seals ``step_n + 1``.
    """

    tenant_id: str
    trajectory_id: str
    work_dir: str | Path
    step_n: int = 1
    effect_hints: Mapping[str, EffectClass] = field(default_factory=dict)
    allow_open_world_override: bool = False
    context: Mapping[str, Any] = field(default_factory=lambda: {"goal": "langgraph-sidecar"})

    _db_path: str = field(init=False, repr=False)
    _traj: Any = field(init=False, repr=False, default=None)
    _sealed: bool = field(init=False, repr=False, default=False)
    _plan_key: str = field(init=False, repr=False, default="")
    _tool_i: int = field(init=False, repr=False, default=0)
    _step_committed: bool = field(init=False, repr=False, default=False)
    _closed: bool = field(init=False, repr=False, default=False)
    _lock: threading.Lock = field(default_factory=threading.Lock, init=False, repr=False)

    def __post_init__(self) -> None:
        root = Path(self.work_dir)
        root.mkdir(parents=True, exist_ok=True)
        self._db_path = str(root / "nodes.sqlite")
        sink = from_env()
        self._traj = open_trajectory(
            self.tenant_id,
            self.trajectory_id,
            db_path=self._db_path,
            console_sink=sink,
        )
        project(self._traj, self.step_n, dict(self.context))

    def close(self) -> None:
        if self._closed:
            return
        self._closed = True
        if self._traj is not None:
            self._traj.close()

    def __enter__(self) -> TrajIRToolGuard:
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()

    def _effect_for(self, name: str) -> EffectClass:
        if name in self.effect_hints:
            return self.effect_hints[name]
        return classify_tool(name, {})

    def _commit_open_step(self) -> None:
        """Caller holds ``_lock``. Skip when this step was already committed."""
        if self._step_committed:
            return
        commit_step(self._traj, self.step_n, seq=2 + 2 * self._tool_i)
        self._step_committed = True

    def _seal_plan(self, tool_calls: Sequence[Mapping[str, Any]]) -> None:
        """Caller holds ``_lock``. Same plan is a no-op. A new plan is a new step."""
        key, plan_calls = _plan_key_and_calls(tool_calls)
        if self._sealed and key == self._plan_key:
            return
        if self._sealed:
            self._commit_open_step()
            next_step = self.step_n + 1
            project(self._traj, next_step, dict(self.context))
            seal_decision(self._traj, next_step, {"tool_calls": plan_calls})
            self.step_n = next_step
            self._tool_i = 0
            self._plan_key = key
            self._sealed = True
            self._step_committed = False
            return
        seal_decision(self._traj, self.step_n, {"tool_calls": plan_calls})
        self._plan_key = key
        self._sealed = True
        self._step_committed = False

    def run_sealed(
        self,
        name: str,
        args: Mapping[str, Any] | None,
        fn: Callable[..., Any],
        *,
        plan_calls: Sequence[Mapping[str, Any]] | None = None,
    ) -> Any:
        """Seal this turn's plan, classify, exec_tool, and return the tool result.

        The lock covers the seal and the tool body so two calls cannot take the
        same sequence number or open the next step while a tool is still running.

        ``fn`` receives kwargs only. The idempotency key is never injected into kwargs.
        """
        if self._closed:
            raise RuntimeError("TrajIRToolGuard is closed")
        calls = (
            list(plan_calls)
            if plan_calls is not None
            else [{"name": name, "args": _as_mapping(args)}]
        )
        with self._lock:
            if self._closed:
                raise RuntimeError("TrajIRToolGuard is closed")
            self._seal_plan(calls)
            effect = self._effect_for(name)
            assert_open_world_effect(
                name,
                effect,
                allow_override=self.allow_open_world_override,
            )
            seq = 2 + 2 * self._tool_i
            self._tool_i += 1
            tool = Tool(
                name=name,
                fn=fn,
                effect_class=effect,
                allow_open_world_override=self.allow_open_world_override,
            )
            result = exec_tool(
                self._traj,
                self.step_n,
                {"args": _as_mapping(args)},
                tool,
                seq=seq,
            )
            return result.result

    def finish(self) -> None:
        """Append COMMIT_STEP for the current step."""
        if self._closed:
            return
        with self._lock:
            if self._closed:
                return
            self._commit_open_step()

    def export_tir(self, dest: str | Path, *, redacted: bool = False) -> str:
        """Export a thin verified package for ``trajir verify``."""
        self.finish()
        dest_path = str(dest)
        log = NodeLog(self._db_path)
        sink = from_env()

        def _on_export(info: dict) -> None:
            note_package(
                sink,
                kind="export.completed",
                trajectory_id=self.trajectory_id,
                tenant_id=self.tenant_id,
                path=str(info["path"]),
                mode=str(info["mode"]),
                redacted=bool(info["redacted"]),
                nbytes=int(info["bytes"]),
                member_count=int(info["member_count"]),
                node_count=int(info["node_count"]),
                ok=bool(info["ok"]),
                error=str(info.get("error") or ""),
            )

        try:
            export_tir(
                log,
                self.trajectory_id,
                dest_path,
                mode="thin",
                tenant_id=self.tenant_id,
                redacted=redacted,
                on_exported=_on_export,
            )
        finally:
            log.close()
        return dest_path

    def wrap_tool_call(self, request: Any, execute: Callable[[Any], Any]) -> Any:
        """LangGraph ToolNode ``wrap_tool_call`` hook.

        Seals before ``execute`` runs the real tool. Requires langgraph only at
        call time (the request/execute objects come from ToolNode).

        When the body runs, its message is returned, including a real ``None``.
        When resume skips the body, the return is a tool message built from the
        sealed result so ToolNode does not see ``None``.
        """
        call = _tool_call_from_request(request)
        name = str(call["name"])
        args = _as_mapping(call.get("args"))

        plan_calls = _plan_calls_from_request(request)
        if not plan_calls:
            plan_calls = [{"name": name, "args": args}]

        held: dict[str, Any] = {}

        def body(**kwargs: Any) -> Any:
            # kwargs come from exec_tool; must match sealed args, not inject keys.
            msg = execute(request)
            held["ran"] = True
            held["msg"] = msg
            return _tool_message_content(msg)

        result = self.run_sealed(name, args, body, plan_calls=plan_calls)
        if held.get("ran"):
            return held.get("msg")
        return _resumed_tool_message(request, result)


def _tool_call_from_request(request: Any) -> Mapping[str, Any]:
    if isinstance(request, Mapping):
        return request
    tc = getattr(request, "tool_call", None)
    if isinstance(tc, Mapping):
        return tc
    if tc is not None and hasattr(tc, "get"):
        return tc  # type: ignore[return-value]
    raise TypeError(f"cannot read tool_call from request type {type(request)!r}")


def _plan_calls_from_request(request: Any) -> list[dict[str, Any]]:
    """Best-effort: pull sibling tool_calls from the AI message on the request."""
    state = getattr(request, "state", None)
    if not isinstance(state, Mapping):
        return []
    messages = state.get("messages") or state.get("msgs")
    if not isinstance(messages, list) or not messages:
        return []
    last = messages[-1]
    raw_calls = getattr(last, "tool_calls", None)
    if not raw_calls and isinstance(last, Mapping):
        raw_calls = last.get("tool_calls")
    if not raw_calls:
        return []
    out: list[dict[str, Any]] = []
    for call in raw_calls:
        if isinstance(call, Mapping):
            out.append({"name": str(call.get("name")), "args": _as_mapping(call.get("args"))})
        else:
            out.append(
                {
                    "name": str(getattr(call, "name", "")),
                    "args": _as_mapping(getattr(call, "args", {})),
                }
            )
    return out


def _tool_call_id(request: Any, call: Mapping[str, Any]) -> str:
    raw = call.get("id")
    if raw:
        return str(raw)
    for attr in ("tool_call_id", "id"):
        value = getattr(request, attr, None)
        if value:
            return str(value)
    return ""


def _resumed_tool_message(request: Any, result: Any) -> Any:
    """Tool message for a resume that did not run the tool body."""
    call = _tool_call_from_request(request)
    tool_call_id = _tool_call_id(request, call)
    content = result if isinstance(result, str) else json.dumps(result, default=str)
    try:
        from langchain_core.messages import ToolMessage
    except ImportError:
        return SimpleNamespace(content=content, tool_call_id=tool_call_id, type="tool")
    try:
        return ToolMessage(content=content, tool_call_id=tool_call_id)
    except Exception:
        return SimpleNamespace(content=content, tool_call_id=tool_call_id, type="tool")


def _tool_message_content(msg: Any) -> Any:
    if msg is None:
        return None
    if isinstance(msg, Mapping):
        return msg.get("content", msg)
    return getattr(msg, "content", msg)


def console_sink_configured() -> bool:
    return bool(os.environ.get("TRAJIR_CONSOLE_SINK", "").strip())
