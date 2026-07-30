"""Conformance test for R01: model inference not reinvoked after decision seal."""
import os

from test.e2e._util import cleanup, hard_kill, read_counter, run_agent, start_agent, wait_for_marker

ARTIFACTS = (
    "test_model_call_count.txt",
    "test_deploy_side_effect_count.txt",
    "decision_sealed.marker",
    "tool_started.marker",
    "kill_mid_deploy.sqlite",
)


def test_r01_no_reinfer_after_seal(tmp_path):
    """R01 conformance: model inference not reinvoked after decision seal.

    Scenario: crash after DECISION seal but before tool execution completes,
    then resume. Model inference should be called exactly once total (not
    re-invoked on resume).
    """
    workdir = str(tmp_path)

    # Clean up any artifacts from prior runs
    cleanup(*[os.path.join(workdir, p) for p in ARTIFACTS])

    # Launch the agent and wait for the decision seal marker
    proc = start_agent(
        "--crash-after=decision_sealed",
        cwd=workdir,
        log_path=os.path.join(workdir, "crash_run.log"),
    )
    wait_for_marker(os.path.join(workdir, "decision_sealed.marker"))
    hard_kill(proc)

    # Resume the agent
    run_agent("--resume", cwd=workdir)

    # Assert model was invoked exactly once across both runs
    model_count = read_counter(os.path.join(workdir, "test_model_call_count.txt"))
    assert model_count == 1, f"model invoked {model_count} times, expected 1 across resume"
