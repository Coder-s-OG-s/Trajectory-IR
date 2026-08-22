import os
import subprocess
import sys
import time

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
AGENT_SCRIPT = os.path.join(REPO_ROOT, "examples", "kill-mid-deploy", "agent.py")

# Every file the fixture agent writes into its working directory, including the
# DBOS system database (named after the agent's app name). A resume run must see
# the crashed run's databases, so scenarios isolate themselves by working
# directory rather than by deleting these between the crash and the resume.
ARTIFACTS = (
    "test_model_call_count.txt",
    "test_deploy_side_effect_count.txt",
    "decision_sealed.marker",
    "tool_started.marker",
    "inference_started.marker",
    "kill_mid_deploy.sqlite",
    "kill-mid-deploy.sqlite",
)


def agent_cmd(*args: str) -> list:
    """Command line for the fixture agent. Uses the current interpreter so the
    subprocess gets the same virtualenv the test is running in."""
    return [sys.executable, AGENT_SCRIPT, *args]


def start_agent(*args: str, cwd: str, log_path: str) -> subprocess.Popen:
    """Launch the agent as a real child process, to be hard-killed later.

    Output goes to a file rather than a pipe: a hard-killed process's piped
    output is lost, and these runs are killed by design.

    The log file must remain open for the entire lifetime of the child process
    (it is the process's stdout fd), so it cannot be managed with a plain
    ``with`` block inside this function.  Instead we guard with try/except: if
    ``subprocess.Popen`` raises for any reason we close the file immediately so
    no descriptor is leaked.  On the happy path the handle is stored on the
    Popen object and closed by :func:`hard_kill` after the process is reaped.
    """
    log = open(log_path, "w", encoding="utf-8")  # noqa: SIM115
    try:
        proc = subprocess.Popen(agent_cmd(*args), cwd=cwd, stdout=log, stderr=subprocess.STDOUT)
    except Exception:
        log.close()
        raise
    proc._log_file = log  # type: ignore[attr-defined]  # keep the handle alive until the process is reaped
    return proc


def run_agent(*args: str, cwd: str, timeout: float = 120.0) -> subprocess.CompletedProcess:
    """Run the agent to completion, capturing its output."""
    return subprocess.run(
        agent_cmd(*args),
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )


def hard_kill(proc: subprocess.Popen) -> None:
    """Abruptly terminate a process to simulate a real crash. Popen.kill()
    already maps to SIGKILL on POSIX and TerminateProcess on Windows -- both
    are non-graceful hard kills, so no platform branching is needed."""
    proc.kill()
    proc.wait()
    log = getattr(proc, "_log_file", None)
    if log is not None:
        log.close()


def wait_for_marker(path: str, timeout: float = 60.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        if os.path.exists(path):
            return
        time.sleep(0.05)
    raise TimeoutError(f"marker file {path!r} did not appear within {timeout}s")


def read_counter(path: str) -> int:
    if not os.path.exists(path):
        return 0
    with open(path, encoding="utf-8") as f:
        return int(f.read().strip() or "0")


def cleanup(*paths: str) -> None:
    for p in paths:
        if os.path.exists(p):
            os.remove(p)
