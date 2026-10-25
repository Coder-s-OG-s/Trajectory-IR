"""Device check for a target attach. Not a week of live Claude use.

Creates a temp project, runs the hook launcher as a subprocess, crashes
before PostToolUse, confirms the gated retry exits 2, exports a .tir,
verifies it, flips one byte, and confirms verify fails. If 127.0.0.1:8787
is already open, it is left alone. If it is free, this process starts the
console and then stops only the process it started.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[1]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.attach.console_launch import console_is_up, start_console_if_free  # noqa: E402
from integrations.attach.project_config import write_local_config  # noqa: E402
from integrations.langgraph.verify_pack import ensure_trajir_built, run_verify  # noqa: E402

from trajectory_ir.package import export_tir  # noqa: E402
from trajectory_ir.runtime.log import NodeLog  # noqa: E402

_PRE = _ROOT / "integrations" / "claude" / "pre_tool_use.py"
_POST = _ROOT / "integrations" / "claude" / "post_tool_use.py"
_LAUNCH = _ROOT / "integrations" / "attach" / "launch_hook.py"


def _env() -> dict[str, str]:
    env = dict(os.environ)
    for key in (
        "TRAJIR_WORK_DIR",
        "TRAJIR_TENANT_ID",
        "TRAJIR_TRAJECTORY_ID",
        "TRAJIR_IDENTITY_FILE",
    ):
        env.pop(key, None)
    return env


def _hook(identity: Path, script: Path, kind: str, event: dict) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, str(_LAUNCH), str(identity), str(script), kind],
        input=json.dumps(event),
        text=True,
        capture_output=True,
        env=_env(),
        check=False,
    )


def _crash_then_block(project: Path) -> None:
    code = write_local_config(
        _ROOT, sys.executable, _ROOT / "go" / "trajir-mcp.exe", target=project
    )
    if code != 0:
        raise RuntimeError("config write failed for the crash project")
    ident_path = project / ".local-evidence" / "claude-one-step.json"
    ident = json.loads(ident_path.read_text(encoding="utf-8"))
    work = Path(ident["work_dir"])
    event = {
        "tool_name": "Bash",
        "tool_use_id": "crash-1",
        "tool_input": {"command": "echo hi"},
    }
    first = _hook(ident_path, _PRE, "pre", event)
    if first.returncode != 0:
        raise RuntimeError(f"first pre exited {first.returncode}: {first.stderr}")
    # Crash stands in for a host that dies before PostToolUse. No record call.
    second = _hook(ident_path, _PRE, "pre", event)
    if second.returncode != 2:
        raise RuntimeError(f"replay pre exited {second.returncode}: {second.stderr}")
    state = json.loads((work / "hook_state.json").read_text(encoding="utf-8"))
    if state["next_step"] != 2:
        raise RuntimeError(f"next_step moved on replay: {state['next_step']}")
    if "replay" in state["pending"]["crash-1"]:
        raise RuntimeError("replay flag was stored")
    log = NodeLog(str(work / "nodes.sqlite"))
    try:
        rows = log.list_nodes(ident["trajectory_id"], tenant_id=ident["tenant_id"])
    finally:
        log.close()
    decisions = [row for row in rows if row["kind"] == "DECISION"]
    if len(decisions) != 1:
        raise RuntimeError(f"expected one decision, saw {len(decisions)}")
    print("crash_resume=ok")


def _seal_export_flip(project: Path) -> None:
    code = write_local_config(
        _ROOT, sys.executable, _ROOT / "go" / "trajir-mcp.exe", target=project
    )
    if code != 0:
        raise RuntimeError("config write failed for the verify project")
    ident_path = project / ".local-evidence" / "claude-one-step.json"
    ident = json.loads(ident_path.read_text(encoding="utf-8"))
    work = Path(ident["work_dir"])
    event = {
        "tool_name": "lookup",
        "tool_use_id": "live-1",
        "tool_input": {"q": "one"},
        "annotations": {"readOnlyHint": True, "destructiveHint": False},
    }
    pre = _hook(ident_path, _PRE, "pre", event)
    if pre.returncode != 0:
        raise RuntimeError(f"verify pre exited {pre.returncode}: {pre.stderr}")
    # The host ran the tool. This script is that host. The hook did not call it.
    post_event = {"tool_use_id": "live-1", "tool_response": {"q": "one"}}
    post = _hook(ident_path, _POST, "post", post_event)
    if post.returncode != 0:
        raise RuntimeError(f"post exited {post.returncode}: {post.stderr}")
    replay = _hook(ident_path, _PRE, "pre", event)
    if replay.returncode != 0:
        raise RuntimeError(f"read-only replay exited {replay.returncode}: {replay.stderr}")
    dest = work / "live.tir"
    log = NodeLog(str(work / "nodes.sqlite"))
    try:
        export_tir(
            log,
            ident["trajectory_id"],
            dest,
            mode="thin",
            tenant_id=ident["tenant_id"],
        )
    finally:
        log.close()
    ensure_trajir_built(_ROOT)
    ok = run_verify(dest, root=_ROOT)
    if not ok.get("ok"):
        raise RuntimeError(f"verify failed on a fresh pack: {ok}")
    blob = bytearray(dest.read_bytes())
    if len(blob) < 2:
        raise RuntimeError("pack was too small to flip")
    blob[len(blob) // 2] ^= 0xFF
    dest.write_bytes(bytes(blob))
    bad = run_verify(dest, root=_ROOT)
    if bad.get("ok"):
        raise RuntimeError("verify accepted a flipped pack")
    print("verify_flip=ok")


def _console_check() -> None:
    if console_is_up():
        print("console=already on 127.0.0.1:8787; left it alone")
        return
    data = Path(tempfile.mkdtemp(prefix="trajir-console-"))
    launch = start_console_if_free(_ROOT, data_dir=data, detach=False)
    process = launch.process
    try:
        if launch.status != "started" or process is None:
            raise RuntimeError(f"console did not start: {launch.status}")
        deadline = time.time() + 15
        while time.time() < deadline:
            if console_is_up():
                print("console=started on 127.0.0.1:8787 and answered")
                return
            if process.poll() is not None:
                break
            time.sleep(0.2)
        raise RuntimeError("console process did not accept 127.0.0.1:8787")
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
        shutil.rmtree(data, ignore_errors=True)


def main() -> int:
    print("week of live Claude use: not run")
    parent = Path(tempfile.mkdtemp(prefix="trajir-attach-"))
    try:
        _crash_then_block(parent / "crash")
        _seal_export_flip(parent / "verify")
        _console_check()
    except Exception as exc:
        print(f"local attach session failed: {exc}", file=sys.stderr)
        return 1
    finally:
        shutil.rmtree(parent, ignore_errors=True)
    print("LOCAL ATTACH OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
