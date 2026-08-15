"""Kill-mid-deploy fixture agent.

Runs one durable step: infer a plan, seal the decision, execute a fake
`deploy_server` tool. Used by test/e2e, conformance/, and the kill-mid-deploy
demo -- one script, three consumers, so crash-recovery behavior is only
implemented once.

Run it twice against the same working directory to exercise crash recovery:

    pip install -e .
    python examples/kill-mid-deploy/agent.py --crash-during=tool_call   # then kill -9
    python examples/kill-mid-deploy/agent.py --resume

Crash points, each idling at a marker file so a harness can hard-kill there:
`--crash-during=inference` (before the decision is sealed),
`--crash-after=decision_sealed` (sealed, before the side effect starts),
`--crash-during=tool_call` (inside the non-idempotent side effect).

All artifacts (node log, DBOS system database, counters, markers) are written
relative to the current working directory, so callers isolate runs by cd'ing
into a scratch directory.
"""

import argparse
import os
import sys
import time

from dbos import SetWorkflowID

from drivers.durable_backend.dbos.adapter import init_backend
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.gate import BlockedNeedsGate
from trajectory_ir.resume.step import make_run_step
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool

TRAJECTORY_ID = "kill-mid-deploy-demo"
TENANT_ID = "demo"
DB_PATH = "kill_mid_deploy.sqlite"
MODEL_CALL_COUNT_FILE = "test_model_call_count.txt"
DEPLOY_COUNT_FILE = "test_deploy_side_effect_count.txt"
DECISION_SEALED_MARKER = "decision_sealed.marker"
TOOL_STARTED_MARKER = "tool_started.marker"
INFERENCE_STARTED_MARKER = "inference_started.marker"

# How long a crash-injecting run idles at its crash point, giving an external
# harness time to see the marker file and hard-kill the process.
CRASH_WINDOW_SECONDS = 30

# Set from --crash-during=inference. A module-level flag rather than a closure
# because `model_call` is handed to the durable backend as-is and the backend
# keys a step's memoized output on the function's qualified name: wrapping it
# per-run would change that name and break the memoization scenario 2 asserts.
_CRASH_DURING_INFERENCE = False


def _bump_counter(path: str) -> int:
    if os.path.exists(path):
        with open(path, encoding="utf-8") as f:
            n = int(f.read().strip() or "0")
    else:
        n = 0
    n += 1
    with open(path, "w", encoding="utf-8") as f:
        f.write(str(n))
    return n


def model_call(context: dict) -> dict:
    _bump_counter(MODEL_CALL_COUNT_FILE)
    print("INFERENCE: model_call invoked", flush=True)
    if _CRASH_DURING_INFERENCE:
        # Crash point strictly *before* the DECISION seal: inference has run but
        # its result has not been returned, so the durable backend has not
        # memoized this step and no DECISION node exists yet.
        with open(INFERENCE_STARTED_MARKER, "w") as f:
            f.write("started")
        time.sleep(CRASH_WINDOW_SECONDS)  # window for the harness to hard-kill us
    return {"tool_calls": [{"name": "deploy_server", "args": {"version": "1.0.0"}}]}


def deploy_server(version: str, crash_during: bool = False) -> dict:
    with open(TOOL_STARTED_MARKER, "w") as f:
        f.write("started")
    print("TOOL_CALL: deploy_server started", flush=True)
    if crash_during:
        time.sleep(CRASH_WINDOW_SECONDS)  # window for the harness to hard-kill us
    _bump_counter(DEPLOY_COUNT_FILE)
    return {"deployed": version}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--resume", action="store_true")
    parser.add_argument("--crash-after", choices=["decision_sealed"], default=None)
    parser.add_argument("--crash-during", choices=["inference", "tool_call"], default=None)
    args = parser.parse_args()

    global _CRASH_DURING_INFERENCE
    _CRASH_DURING_INFERENCE = args.crash_during == "inference"

    if not args.resume:
        for marker in (
            DECISION_SEALED_MARKER,
            TOOL_STARTED_MARKER,
            INFERENCE_STARTED_MARKER,
        ):
            if os.path.exists(marker):
                os.remove(marker)

    node_log = NodeLog(DB_PATH)

    def deploy_wrapper(version: str) -> dict:
        return deploy_server(version, crash_during=(args.crash_during == "tool_call"))

    tool_registry = {
        "deploy_server": Tool(
            name="deploy_server",
            fn=deploy_wrapper,
            effect_class=EffectClass.NON_IDEMPOTENT_WRITE,
        ),
    }

    def seal_marker_hook():
        if args.crash_after == "decision_sealed":
            with open(DECISION_SEALED_MARKER, "w") as f:
                f.write("sealed")
            print("DECISION sealed", flush=True)
            time.sleep(CRASH_WINDOW_SECONDS)  # window for the harness to hard-kill us

    # The workflow must be registered *before* the durable backend launches:
    # DBOS looks up pending workflows for recovery at launch time and resolves
    # them by registered name, and it derives the application version (which
    # pending-workflow lookup is keyed on) from the registered workflows.
    run_step = make_run_step(
        node_log,
        TENANT_ID,
        TRAJECTORY_ID,
        tool_registry,
        on_decision_sealed=seal_marker_hook,
    )

    init_backend(app_name="kill-mid-deploy")

    try:
        with SetWorkflowID(TRAJECTORY_ID):
            results = run_step(step_n=1, model_call=model_call, context={})
    except BlockedNeedsGate as e:
        print(f"BLOCKED_NEEDS_GATE: {e}", flush=True)
        sys.exit(0)

    if args.resume:
        print("Resumed. deploy_server executed exactly once.", flush=True)
    print(f"results={results}", flush=True)


if __name__ == "__main__":
    main()
