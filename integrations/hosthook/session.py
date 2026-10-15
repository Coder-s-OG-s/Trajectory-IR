"""Cross-process seal-then-record session for an external agent host.

Pre-tool: write PROJECT_CONTEXT and DECISION, then return. The host runs the
tool after that. Post-tool: write TOOL_CALL and TOOL_RESULT for the result the
host already produced. The function passed to exec_tool only returns that
result. It is not the real tool.

A crash after the seal leaves the decision in the log. The next tool is a new
step. This is not exactly-once.
"""

from __future__ import annotations

import json
import os
import time
from pathlib import Path
from typing import Any

from client.python.trajectory_client import (
    commit_step,
    exec_tool,
    open_trajectory,
    project,
    seal_decision,
)
from trajectory_ir.effects import EffectClass, classify_tool
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import payload_hash
from trajectory_ir.runtime.tool import Tool


def _jsonable(value: Any) -> Any:
    return json.loads(json.dumps(value, default=str))


def _state_path(work_dir: Path) -> Path:
    return work_dir / "hook_state.json"


def _empty_state() -> dict[str, Any]:
    return {"next_step": 1, "pending": {}, "done": {}}


def _load_state(work_dir: Path) -> dict[str, Any]:
    path = _state_path(work_dir)
    if not path.is_file():
        return _empty_state()
    data = json.loads(path.read_text(encoding="utf-8"))
    data.setdefault("next_step", 1)
    data.setdefault("pending", {})
    data.setdefault("done", {})
    return data


def _save_state(work_dir: Path, state: dict[str, Any]) -> None:
    path = _state_path(work_dir)
    tmp = path.with_suffix(".json.tmp")
    tmp.write_text(json.dumps(state, sort_keys=True), encoding="utf-8")
    os.replace(tmp, path)


class _Lock:
    def __init__(self, work_dir: Path) -> None:
        self.work_dir = work_dir
        self.fd: int | None = None

    def __enter__(self) -> _Lock:
        self.work_dir.mkdir(parents=True, exist_ok=True)
        lock_path = self.work_dir / "hook.lock"
        deadline = time.time() + 15
        while True:
            try:
                self.fd = os.open(str(lock_path), os.O_CREAT | os.O_EXCL | os.O_RDWR)
                return self
            except FileExistsError:
                try:
                    age = time.time() - lock_path.stat().st_mtime
                except OSError:
                    age = 0
                if age > 15:
                    try:
                        lock_path.unlink()
                    except OSError:
                        pass
                    continue
                if time.time() > deadline:
                    raise TimeoutError(f"trajir hook lock held: {lock_path}")
                time.sleep(0.05)

    def __exit__(self, *args: Any) -> None:
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None
        try:
            (self.work_dir / "hook.lock").unlink()
        except OSError:
            pass


def _decision_facts(work_dir: Path, tenant_id: str, trajectory_id: str, step_n: int) -> dict[str, str]:
    log = NodeLog(str(work_dir / "nodes.sqlite"))
    try:
        rows = log.list_nodes(trajectory_id, tenant_id=tenant_id)
    finally:
        log.close()
    for row in rows:
        if row.get("step_n") == step_n and row.get("kind") == "DECISION":
            payload = row.get("payload") if isinstance(row.get("payload"), dict) else {}
            return {
                "decision_node_id": str(row.get("id") or ""),
                "content_hash": payload_hash(payload) if payload else "",
            }
    return {"decision_node_id": "", "content_hash": ""}


def _label(work_dir: Path, tenant_id: str, trajectory_id: str, step_n: int, tool_name: str, effect: str) -> None:
    from integrations.otel.correlate import correlate_enabled, current_span, label_after_seal

    if not correlate_enabled():
        return
    facts = _decision_facts(work_dir, tenant_id, trajectory_id, step_n)
    label_after_seal(
        current_span(),
        decision_node_id=facts["decision_node_id"],
        trajectory_id=trajectory_id,
        tenant_id=tenant_id,
        step_n=step_n,
        content_hash=facts["content_hash"],
        effect_class=effect,
        tool_name=tool_name,
    )


def seal_before_tool(
    work_dir: str | Path,
    tenant_id: str,
    trajectory_id: str,
    tool_name: str,
    args: dict[str, Any] | None,
    tool_use_id: str,
    annotations: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Seal one tool call. Do not run the tool."""
    if not tool_use_id:
        raise ValueError("tool_use_id is required")
    root = Path(work_dir)
    safe_args = _jsonable(args or {})
    effect = classify_tool(tool_name, annotations)
    with _Lock(root):
        state = _load_state(root)
        existing = state["pending"].get(tool_use_id) or state["done"].get(tool_use_id)
        if existing:
            return dict(existing)
        step_n = int(state["next_step"])
        db = str(root / "nodes.sqlite")
        traj = open_trajectory(tenant_id, trajectory_id, db_path=db)
        try:
            project(traj, step_n, {"goal": "host-hook", "tool": tool_name})
            seal_decision(
                traj,
                step_n,
                {"tool_calls": [{"name": tool_name, "args": safe_args}]},
            )
        finally:
            traj.close()
        record = {
            "tool_use_id": tool_use_id,
            "step_n": step_n,
            "tool_name": tool_name,
            "args": safe_args,
            "effect_class": effect.value,
            "sealed": True,
        }
        state["pending"][tool_use_id] = record
        state["next_step"] = step_n + 1
        _save_state(root, state)
    _label(root, tenant_id, trajectory_id, step_n, tool_name, effect.value)
    return record


def record_tool_result(
    work_dir: str | Path,
    tenant_id: str,
    trajectory_id: str,
    tool_use_id: str,
    result: Any,
) -> dict[str, Any]:
    """Record the host's result for a seal that already happened.

    The body passed to exec_tool returns ``result``. It does not call the tool.
    """
    if not tool_use_id:
        raise ValueError("tool_use_id is required")
    root = Path(work_dir)
    safe_result = _jsonable(result)
    with _Lock(root):
        state = _load_state(root)
        if tool_use_id in state["done"]:
            return dict(state["done"][tool_use_id])
        pending = state["pending"].get(tool_use_id)
        if not pending:
            raise KeyError(f"no sealed call for tool_use_id {tool_use_id}")
        step_n = int(pending["step_n"])
        effect = EffectClass(pending["effect_class"])
        args = pending["args"]
        name = str(pending["tool_name"])

        def already_produced(**_kwargs: Any) -> Any:
            return safe_result

        db = str(root / "nodes.sqlite")
        traj = open_trajectory(tenant_id, trajectory_id, db_path=db)
        try:
            exec_tool(
                traj,
                step_n,
                {"args": args},
                Tool(name=name, fn=already_produced, effect_class=effect),
                seq=2,
            )
            commit_step(traj, step_n, seq=4)
        finally:
            traj.close()
        done = dict(pending)
        done["recorded"] = True
        state["done"][tool_use_id] = done
        del state["pending"][tool_use_id]
        _save_state(root, state)
        return done
