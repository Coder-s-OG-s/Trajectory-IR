"""Cross-process seal-then-record session for an external agent host.

Pre-tool: write PROJECT_CONTEXT and DECISION through NodeLog, then return.
The host runs the tool after that. Post-tool: write TOOL_CALL and TOOL_RESULT
for the result the host already produced. The function passed to the recorder
only returns that result. It is not the real tool.

A failed host tool writes TOOL_CALL, then ABORT with reason
HOST_REPORTED_FAILURE, then COMMIT_STEP. The pending row is cleared. The tool
is not run again.

This path does not open the durable backend. A crash after the seal leaves the
decision in the log. The next tool is a new step. This is not exactly-once.
On this path TOOL_CALL is written after the host tool, so it records the call
and does not gate it.
"""

from __future__ import annotations

import contextlib
import json
import os
import time
from pathlib import Path
from typing import Any

from trajectory_ir.effects.classify import (
    EffectClass,
    assert_open_world_effect,
    classify_tool,
    requires_block_and_gate,
)
from trajectory_ir.resume.gate import make_gated_tool_call, make_plain_tool_call
from trajectory_ir.resume.idempotency import CallMeta
from trajectory_ir.resume.world import decision_payload
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.nodes import payload_hash

_TOOL_SEQ = 2
_COMMIT_SEQ = 4
_ERROR_TEXT_LIMIT = 2000
_HOST_FAILURE = "HOST_REPORTED_FAILURE"


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
    loaded = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(loaded, dict):
        raise ValueError("hook_state.json must be an object")
    data: dict[str, Any] = {str(key): value for key, value in loaded.items()}
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
                    with contextlib.suppress(OSError):
                        lock_path.unlink()
                    continue
                if time.time() > deadline:
                    raise TimeoutError(f"trajir hook lock held: {lock_path}") from None
                time.sleep(0.05)

    def __exit__(self, *args: Any) -> None:
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None
        with contextlib.suppress(OSError):
            (self.work_dir / "hook.lock").unlink()


def _decision_facts(
    work_dir: Path, tenant_id: str, trajectory_id: str, step_n: int
) -> dict[str, str]:
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


def _label(
    work_dir: Path,
    tenant_id: str,
    trajectory_id: str,
    step_n: int,
    tool_name: str,
    effect: str,
) -> None:
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


def _error_text(error: Any) -> str:
    if error is None or error == "":
        return "host reported a tool failure"
    text = error if isinstance(error, str) else json.dumps(_jsonable(error), sort_keys=True)
    if len(text) > _ERROR_TEXT_LIMIT:
        return text[:_ERROR_TEXT_LIMIT]
    return text


def _tool_payload(
    tenant_id: str,
    trajectory_id: str,
    step_n: int,
    tool_name: str,
    args: dict[str, Any],
) -> dict[str, Any]:
    meta = CallMeta.make(tenant_id, trajectory_id, step_n, _TOOL_SEQ)
    return {"tool": tool_name, "args": args, "idempotency_key": meta.idempotency_key}


def _outcome_exists(log: NodeLog, trajectory_id: str, tenant_id: str, step_n: int) -> bool:
    seq = _TOOL_SEQ + 1
    return log.has(trajectory_id, tenant_id, step_n, "TOOL_RESULT", seq=seq) or log.has(
        trajectory_id, tenant_id, step_n, "ABORT", seq=seq
    )


def _mark_failed(
    pending: dict[str, Any], *, recorded: bool, already_claimed: bool
) -> dict[str, Any]:
    done = dict(pending)
    done["recorded"] = recorded
    done["failed"] = True
    done["already_claimed"] = already_claimed
    return done


def _close_pending(
    work_dir: Path, state: dict[str, Any], tool_use_id: str, done: dict[str, Any]
) -> None:
    state["done"][tool_use_id] = done
    del state["pending"][tool_use_id]
    _save_state(work_dir, state)


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
        log = NodeLog(str(root / "nodes.sqlite"))
        try:
            plan = {"tool_calls": [{"name": tool_name, "args": safe_args}]}
            log.append(
                "PROJECT_CONTEXT",
                step_n,
                {"goal": "host-hook", "tool": tool_name},
                trajectory_id,
                tenant_id,
                seq=0,
            )
            log.append(
                "DECISION",
                step_n,
                decision_payload(plan),
                trajectory_id,
                tenant_id,
                seq=1,
            )
        finally:
            log.close()
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

    The body passed to the recorder returns ``result``. It does not call the tool.
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
        assert_open_world_effect(name, effect)

        def already_produced(**_kwargs: Any) -> Any:
            return safe_result

        log = NodeLog(str(root / "nodes.sqlite"))
        try:
            if requires_block_and_gate(effect):
                call = make_gated_tool_call(
                    log,
                    trajectory_id,
                    tenant_id,
                    step_n,
                    seq=_TOOL_SEQ,
                    tool_name=name,
                    tool_fn=already_produced,
                )
            else:
                call = make_plain_tool_call(
                    log,
                    trajectory_id,
                    tenant_id,
                    step_n,
                    seq=_TOOL_SEQ,
                    tool_name=name,
                    tool_fn=already_produced,
                )
            call(**args)
            log.append("COMMIT_STEP", step_n, {}, trajectory_id, tenant_id, seq=_COMMIT_SEQ)
        finally:
            log.close()
        done = dict(pending)
        done["recorded"] = True
        state["done"][tool_use_id] = done
        del state["pending"][tool_use_id]
        _save_state(root, state)
        return done


def record_tool_failure(
    work_dir: str | Path,
    tenant_id: str,
    trajectory_id: str,
    tool_use_id: str,
    error: Any,
) -> dict[str, Any]:
    """Record a host-reported tool failure and clear the pending row.

    Writes TOOL_CALL, then ABORT reason HOST_REPORTED_FAILURE, then COMMIT_STEP.
    Does not run the tool. If an outcome row is already stored, this does not
    append a second ABORT.
    """
    if not tool_use_id:
        raise ValueError("tool_use_id is required")
    root = Path(work_dir)
    text = _error_text(error)
    with _Lock(root):
        state = _load_state(root)
        if tool_use_id in state["done"]:
            return dict(state["done"][tool_use_id])
        pending = state["pending"].get(tool_use_id)
        if not pending:
            raise KeyError(f"no sealed call for tool_use_id {tool_use_id}")
        step_n = int(pending["step_n"])
        effect = EffectClass(pending["effect_class"])
        args = pending["args"] if isinstance(pending["args"], dict) else {}
        name = str(pending["tool_name"])
        assert_open_world_effect(name, effect)
        log = NodeLog(str(root / "nodes.sqlite"))
        try:
            payload = _tool_payload(tenant_id, trajectory_id, step_n, name, args)
            # An outcome is already stored. Close pending and do not append
            # another ABORT beside it.
            if _outcome_exists(log, trajectory_id, tenant_id, step_n):
                done = _mark_failed(pending, recorded=False, already_claimed=True)
                _close_pending(root, state, tool_use_id, done)
                return done
            if requires_block_and_gate(effect):
                log.claim_tool_call(step_n, payload, trajectory_id, tenant_id, _TOOL_SEQ)
            else:
                log.append(
                    "TOOL_CALL",
                    step_n,
                    payload,
                    trajectory_id,
                    tenant_id,
                    seq=_TOOL_SEQ,
                )
            log.append(
                "ABORT",
                step_n,
                {"reason": _HOST_FAILURE, "tool": name, "error": text},
                trajectory_id,
                tenant_id,
                seq=_TOOL_SEQ + 1,
            )
            log.append(
                "COMMIT_STEP",
                step_n,
                {},
                trajectory_id,
                tenant_id,
                seq=_COMMIT_SEQ,
            )
        finally:
            log.close()
        done = _mark_failed(pending, recorded=True, already_claimed=False)
        _close_pending(root, state, tool_use_id, done)
        return done
