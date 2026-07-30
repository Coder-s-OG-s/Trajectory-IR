"""Conformance test for R01: model inference not reinvoked after decision seal."""
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


def test_r01_no_reinfer_after_seal(tmp_path):
    """R01 conformance: model inference not reinvoked after decision seal.

    Scenario: crash after DECISION seal but before tool execution completes,
    then resume. Model inference should be called exactly once total (not
    re-invoked on resume), and the tool side effect must complete exactly once.
    """
    workdir = str(tmp_path)

    # Launch the agent and wait for the decision seal marker
    proc = start_agent(
        "--crash-after=decision_sealed",
        cwd=workdir,
        log_path=os.path.join(workdir, "crash_run.log"),
    )
    wait_for_marker(os.path.join(workdir, "decision_sealed.marker"))
    hard_kill(proc)

    # Resume the agent
    result = run_agent("--resume", cwd=workdir)

    # Assert resume completed successfully
    assert result.returncode == 0, (
        "resume failed" + _report(workdir, result)
    )

    # Assert model was invoked exactly once across both runs
    model_count = read_counter(os.path.join(workdir, "test_model_call_count.txt"))
    assert model_count == 1, (
        f"model invoked {model_count} times, expected 1 across resume" + _report(workdir, result)
    )

    # Assert tool side effect executed exactly once
    deploy_count = read_counter(os.path.join(workdir, "test_deploy_side_effect_count.txt"))
    assert deploy_count == 1, (
        f"tool side effect ran {deploy_count} times, expected 1" + _report(workdir, result)
    )
