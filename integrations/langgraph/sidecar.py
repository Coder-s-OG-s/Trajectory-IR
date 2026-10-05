"""In-process evidence wrap: seal → classify → hashed key → tool → NodeLog.

This is the control plane TURNING_POINT.md describes. It seals *before*
the tool body runs. A post-hoc OTel collector cannot replace this wrap.
"""

from __future__ import annotations

import os
from collections.abc import Callable, Mapping, MutableMapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path
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


@dataclass
class TrajIRToolGuard:
    """Seal each mutating tool call into a Trajectory IR NodeLog.

    Use with LangGraph::

        guard = TrajIRToolGuard(tenant_id="demo", trajectory_id="lg-1", work_dir=".")
        ToolNode(tools, wrap_tool_call=guard.wrap_tool_call)

    Or call ``run_sealed`` from tests without LangGraph installed.
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
    _tool_i: int = field(init=False, repr=False, default=0)
    _closed: bool = field(init=False, repr=False, default=False)

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

    def _seal_plan(self, tool_calls: Sequence[Mapping[str, Any]]) -> None:
        if self._sealed:
            return
        plan_calls = []
        for c in tool_calls:
            plan_calls.append(
                {
                    "name": str(c["name"]),
                    "args": _as_mapping(c.get("args")),
                }
            )
        seal_decision(self._traj, self.step_n, {"tool_calls": plan_calls})
        self._sealed = True

    def run_sealed(
        self,
        name: str,
        args: Mapping[str, Any] | None,
        fn: Callable[..., Any],
        *,
        plan_calls: Sequence[Mapping[str, Any]] | None = None,
    ) -> Any:
        """Seal (once per step), classify, exec_tool, return tool result.

        ``fn`` receives kwargs only. Idempotency key is never injected into kwargs.
        """
        if self._closed:
            raise RuntimeError("TrajIRToolGuard is closed")
        calls = list(plan_calls) if plan_calls is not None else [{"name": name, "args": _as_mapping(args)}]
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
        commit_seq = 2 + 2 * self._tool_i
        commit_step(self._traj, self.step_n, seq=commit_seq)

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
        """
        call = _tool_call_from_request(request)
        name = str(call["name"])
        args = _as_mapping(call.get("args"))

        plan_calls = _plan_calls_from_request(request)
        if not plan_calls:
            plan_calls = [{"name": name, "args": args}]

        held: MutableMapping[str, Any] = {}

        def body(**kwargs: Any) -> Any:
            # kwargs come from exec_tool; must match sealed args, not inject keys.
            msg = execute(request)
            held["msg"] = msg
            return _tool_message_content(msg)

        self.run_sealed(name, args, body, plan_calls=plan_calls)
        return held.get("msg")


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
    for c in raw_calls:
        if isinstance(c, Mapping):
            out.append({"name": str(c.get("name")), "args": _as_mapping(c.get("args"))})
        else:
            out.append(
                {
                    "name": str(getattr(c, "name", "")),
                    "args": _as_mapping(getattr(c, "args", {})),
                }
            )
    return out


def _tool_message_content(msg: Any) -> Any:
    if msg is None:
        return None
    if isinstance(msg, Mapping):
        return msg.get("content", msg)
    return getattr(msg, "content", msg)


def console_sink_configured() -> bool:
    return bool(os.environ.get("TRAJIR_CONSOLE_SINK", "").strip())
