"""Real-crash e2e tests: a child process is hard-killed (SIGKILL / TerminateProcess)
at a precise point in a durable step, then resumed in a fresh process pinned to
the same durable workflow id.

Each scenario gets its own working directory so the node log and the durable
backend's system database survive the crash but never leak between scenarios.
"""

import os
import sqlite3

from test.e2e._util import (
    hard_kill,
    read_counter,
    run_agent,
    start_agent,
    wait_for_marker,
)

MODEL_COUNT = "test_model_call_count.txt"
DEPLOY_COUNT = "test_deploy_side_effect_count.txt"
DECISION_SEALED_MARKER = "decision_sealed.marker"
TOOL_STARTED_MARKER = "tool_started.marker"
INFERENCE_STARTED_MARKER = "inference_started.marker"
NODE_LOG = "kill_mid_deploy.sqlite"


def _logged_kinds(workdir):
    """Node kinds recorded in the agent's node log, in write order."""
    conn = sqlite3.connect(os.path.join(workdir, NODE_LOG))
    try:
        return [row[0] for row in conn.execute("SELECT kind FROM nodes ORDER BY ts")]
    finally:
        conn.close()


def _report(workdir, result):
    crash_log = os.path.join(workdir, "crash_run.log")
    if os.path.exists(crash_log):
        with open(crash_log, encoding="utf-8") as f:
            crash_output = f.read()
    else:
        crash_output = "<none>"
    return (
        f"\n--- crashed run output ---\n{crash_output}"
        f"\n--- resume run stdout ---\n{result.stdout}"
        f"\n--- resume run stderr ---\n{result.stderr}"
        f"\n--- node log kinds ---\n{_logged_kinds(workdir)}\n"
    )


def test_crash_before_seal_allows_reinference(tmp_path):
    """Scenario 1: crash before DECISION is sealed -> resume -> new
    inference is allowed (cheap, no side effect has happened yet)."""
    workdir = str(tmp_path)

    # Crash *inside* inference, so the durable workflow is genuinely registered
    # and left pending and the resume below is a real crash-recovery replay.
    # Killing right after Popen instead would land before the backend launches,
    # leaving nothing pending and making the "resume" a cold first run.
    with start_agent(
        "--crash-during=inference",
        cwd=workdir,
        log_path=os.path.join(workdir, "crash_run.log"),
    ) as proc:
        wait_for_marker(os.path.join(workdir, INFERENCE_STARTED_MARKER))
        hard_kill(proc)

    # The kill landed in the intended pre-seal window: inference had started but
    # the decision was never sealed, so no DECISION node exists to replay from.
    kinds = _logged_kinds(workdir)
    assert "DECISION" not in kinds, kinds
    assert read_counter(os.path.join(workdir, DEPLOY_COUNT)) == 0
    assert not os.path.exists(os.path.join(workdir, TOOL_STARTED_MARKER))

    result = run_agent("--resume", cwd=workdir)

    assert result.returncode == 0, _report(workdir, result)
    # Nothing was sealed before the crash, so the replay redoes inference and
    # completes the step. Re-inference is deliberately NOT asserted to be
    # once-only here: pre-seal it is cheap and expected. Scenario 2 owns the
    # "model is not re-invoked" invariant, which only applies after the seal.
    assert read_counter(os.path.join(workdir, MODEL_COUNT)) >= 1, _report(workdir, result)
    kinds = _logged_kinds(workdir)
    assert "DECISION" in kinds and "COMMIT_STEP" in kinds, _report(workdir, result)
    # The one invariant that must hold regardless of how often inference ran:
    # the non-idempotent side effect happened exactly once.
    assert read_counter(os.path.join(workdir, DEPLOY_COUNT)) == 1, _report(workdir, result)


def test_crash_after_seal_before_tool_completes_no_reinfer(tmp_path):
    """Scenario 2: crash after DECISION seal but before tool execution
    completes -> resume -> tool executes to seal, model inference is NOT
    called a second time (assert the counter, not just 'tools ran once')."""
    workdir = str(tmp_path)

    with start_agent(
        "--crash-after=decision_sealed",
        cwd=workdir,
        log_path=os.path.join(workdir, "crash_run.log"),
    ) as proc:
        wait_for_marker(os.path.join(workdir, DECISION_SEALED_MARKER))
        hard_kill(proc)

    # The kill landed in the intended window: inference ran and the decision was
    # sealed, but the non-idempotent side effect had not started yet.
    assert read_counter(os.path.join(workdir, MODEL_COUNT)) == 1
    assert read_counter(os.path.join(workdir, DEPLOY_COUNT)) == 0
    assert not os.path.exists(os.path.join(workdir, TOOL_STARTED_MARKER))

    result = run_agent("--resume", cwd=workdir)

    assert result.returncode == 0, _report(workdir, result)
    assert read_counter(os.path.join(workdir, MODEL_COUNT)) == 1, (
        "model was invoked more than once across resume" + _report(workdir, result)
    )
    assert read_counter(os.path.join(workdir, DEPLOY_COUNT)) == 1, _report(workdir, result)
    kinds = _logged_kinds(workdir)
    assert "TOOL_RESULT" in kinds and "COMMIT_STEP" in kinds, _report(workdir, result)


def test_crash_mid_nonidempotent_tool_blocks_not_retries(tmp_path):
    """Scenario 3: crash mid non-idempotent tool -> resume -> call is
    BLOCKED_NEEDS_GATE, nothing auto-retries."""
    workdir = str(tmp_path)

    with start_agent(
        "--crash-during=tool_call",
        cwd=workdir,
        log_path=os.path.join(workdir, "crash_run.log"),
    ) as proc:
        wait_for_marker(os.path.join(workdir, TOOL_STARTED_MARKER))
        hard_kill(proc)

    # The kill landed inside the non-idempotent tool call: it was logged as
    # started but never completed.
    kinds = _logged_kinds(workdir)
    assert "TOOL_CALL" in kinds and "TOOL_RESULT" not in kinds
    assert read_counter(os.path.join(workdir, DEPLOY_COUNT)) == 0

    result = run_agent("--resume", cwd=workdir)

    # Exit 0 plus the agent's *own* printed prefix. A bare "BLOCKED_NEEDS_GATE"
    # substring is not evidence: the same string appears in the durable
    # backend's framework error log when the exception fails to survive its
    # pickle round trip, so the loose match passed even when
    # `except BlockedNeedsGate` never fired and the process died on a TypeError.
    # This asserts the typed exception was reconstructed and caught by the
    # agent, which is the only path that both prints this line and exits 0.
    assert result.returncode == 0, _report(workdir, result)
    assert "BLOCKED_NEEDS_GATE: step 1: 'deploy_server'" in result.stdout + result.stderr, _report(
        workdir, result
    )
    assert read_counter(os.path.join(workdir, DEPLOY_COUNT)) <= 1, (
        "deploy_server side effect ran more than once" + _report(workdir, result)
    )
    # Stronger than <= 1: the interrupted effect was never re-run at all.
    assert read_counter(os.path.join(workdir, DEPLOY_COUNT)) == 0, _report(workdir, result)
    kinds = _logged_kinds(workdir)
    assert "ABORT" in kinds, _report(workdir, result)
    assert "TOOL_RESULT" not in kinds, _report(workdir, result)
    assert "COMMIT_STEP" not in kinds, _report(workdir, result)
