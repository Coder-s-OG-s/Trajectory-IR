"""Minimal agent host loop using the public Trajectory IR client.

Run from the repository root::

    python examples/host_loop/run_host.py
    python examples/host_loop/run_host.py --sandbox

No paid model API is required: ``stub_model`` is a deterministic stand in.
"""

from __future__ import annotations

import argparse
import os
import sys
import tempfile

# Allow ``python examples/host_loop/run_host.py`` without a prior editable install.
_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
for _path in (_REPO_ROOT, os.path.join(_REPO_ROOT, "pkg")):
    if _path not in sys.path:
        sys.path.insert(0, _path)

from client.python.trajectory_client import (  # noqa: E402
    commit_step,
    exec_tool,
    open_trajectory,
    project,
    seal_decision,
)
from trajectory_ir.effects import EffectClass  # noqa: E402
from trajectory_ir.runtime.log import NodeLog  # noqa: E402
from trajectory_ir.runtime.sandbox import SandboxForbidden  # noqa: E402
from trajectory_ir.runtime.tool import Tool  # noqa: E402

TENANT_ID = "demo"
TRAJECTORY_ID = "host-loop-demo"


def stub_model(context: dict) -> dict:
    """Stand in for an LLM. Returns a sealed plan with concrete args only."""
    name = context.get("user_name", "world")
    return {
        "tool_calls": [
            {"name": "greet", "args": {"name": name}},
            {"name": "deploy_server", "args": {"version": "1.0.0"}},
        ]
    }


def make_tools() -> dict[str, Tool]:
    def greet(*, name: str) -> str:
        return f"hello, {name}"

    def deploy_server(*, version: str) -> str:
        return f"deployed:{version}"

    return {
        "greet": Tool(
            name="greet",
            fn=greet,
            effect_class=EffectClass.PURE,
        ),
        "deploy_server": Tool(
            name="deploy_server",
            fn=deploy_server,
            effect_class=EffectClass.NON_IDEMPOTENT_WRITE,
        ),
    }


def run_one_step(
    *,
    db_path: str,
    model_call=stub_model,
    sandbox: bool = False,
) -> list[object]:
    """Execute one host owned step. Returns tool results in seal order."""
    mode = "sandbox" if sandbox else "live"
    traj = open_trajectory(
        TENANT_ID,
        TRAJECTORY_ID,
        db_path=db_path,
        mode=mode,
    )
    context = {"user_name": "trajectory", "goal": "greet then deploy"}
    project(traj, step_n=1, context=context)

    plan = model_call(context)
    seal_decision(traj, step_n=1, plan=plan)

    tools = make_tools()
    results: list[object] = []
    for i, call in enumerate(plan["tool_calls"]):
        tool = tools[call["name"]]
        # seq = 2 + 2*i keeps TOOL_CALL and TOOL_RESULT slots non overlapping.
        seq = 2 + 2 * i
        result = exec_tool(
            traj,
            step_n=1,
            call={"args": call["args"]},
            tool=tool,
            seq=seq,
        )
        results.append(result.result)

    # After the last tool pair: seq of last TOOL_RESULT is (2+2*(n-1))+1 = 2*n+1
    # Commit uses the next free seq: 2 + 2*n
    commit_seq = 2 + 2 * len(plan["tool_calls"])
    commit_step(traj, step_n=1, seq=commit_seq)
    return results


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Trajectory IR host loop example")
    parser.add_argument(
        "--sandbox",
        action="store_true",
        help="Open trajectory in sandbox mode (rejects NON_IDEMPOTENT_WRITE)",
    )
    parser.add_argument(
        "--db",
        default="",
        help="SQLite path for the IR log (default: temp file)",
    )
    args = parser.parse_args(argv)

    if args.db:
        db_path = args.db
        cleanup = False
    else:
        fd, db_path = tempfile.mkstemp(suffix=".sqlite")
        os.close(fd)
        cleanup = True

    try:
        if args.sandbox:
            try:
                run_one_step(db_path=db_path, sandbox=True)
            except SandboxForbidden as exc:
                print(f"sandbox correctly rejected non idempotent tool: {exc}")
                return 0
            print("error: sandbox should have rejected deploy_server", file=sys.stderr)
            return 1

        results = run_one_step(db_path=db_path, sandbox=False)
        log = NodeLog(db_path)
        kinds = [n["kind"] for n in log.list_nodes(TRAJECTORY_ID, tenant_id=TENANT_ID)]
        print("host step complete")
        print(f"  tool results: {results}")
        print(f"  node kinds:   {kinds}")
        log.close()
        return 0
    finally:
        if cleanup:
            try:
                os.remove(db_path)
            except OSError:
                pass


if __name__ == "__main__":
    raise SystemExit(main())
