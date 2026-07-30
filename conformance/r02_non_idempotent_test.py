"""Conformance test for R02: non-idempotent tool crash blocks, not retries."""
import os

from test.e2e._util import hard_kill, read_counter, run_agent, start_agent, wait_for_marker


def _report(workdir, result):
    """Diagnostic helper for resume run failures."""
    crash_log = os.path.join(workdir, "crash_run.log")
    crash_output = open(crash_log).read() if os.path.exists(crash_log) else "<none>"
    return (
        f"\n--- crashed run output ---\n{crash_output}"
        f"\n--- resume run stdout ---\n{result.stdout}"
        f"\n--- resume run stderr ---\n{result.stderr}\n"
    )


def test_r02_crash_mid_tool_blocks_not_retries(tmp_path):
    """R02 conformance: crash mid non-idempotent tool -> resume -> BLOCKED_NEEDS_GATE, no retries.

    Scenario: crash during tool execution (mid-flight), then resume.
    The tool call must be blocked (not auto-retried), raising BlockedNeedsGate,
    the side effect must never execute (deploy_count == 0), and the agent must
    exit 0 after catching the exception.
    """
    workdir = str(tmp_path)

    # Launch the agent and wait for the tool to start
    proc = start_agent(
        "--crash-during=tool_call",
        cwd=workdir,
        log_path=os.path.join(workdir, "crash_run.log"),
    )
    wait_for_marker(os.path.join(workdir, "tool_started.marker"))
    hard_kill(proc)

    # Resume the agent
    result = run_agent("--resume", cwd=workdir)

    # Assert resume completed successfully (exit 0) and the exception was caught
    # by the agent's except BlockedNeedsGate block (the only path that exits 0
    # and prints the exact block message with the correct prefix).
    assert result.returncode == 0, (
        "resume failed or exception not caught" + _report(workdir, result)
    )
    assert "BLOCKED_NEEDS_GATE: step 1: 'deploy_server'" in result.stdout + result.stderr, (
        "resume did not report BLOCKED_NEEDS_GATE with correct prefix" + _report(workdir, result)
    )

    # Assert the interrupted side effect never ran at all (strictly 0)
    deploy_count = read_counter(os.path.join(workdir, "test_deploy_side_effect_count.txt"))
    assert deploy_count == 0, (
        f"deploy_server side effect ran {deploy_count} times, expected 0" + _report(workdir, result)
    )
