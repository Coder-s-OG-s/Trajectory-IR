import os
import subprocess
import sys
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def _run_without_dbos(code: str) -> subprocess.CompletedProcess[str]:
    # sys.modules['dbos'] = None makes `import dbos` fail as if it were not installed.
    env = {**os.environ, "PYTHONPATH": os.pathsep.join([str(ROOT / "pkg"), str(ROOT)])}
    prelude = "import sys\nsys.modules['dbos'] = None\n"
    return subprocess.run(
        [sys.executable, "-c", prelude + code],
        capture_output=True,
        text=True,
        cwd=ROOT,
        env=env,
    )


def test_dbos_is_an_extra_not_a_core_dependency():
    with open(ROOT / "pyproject.toml", "rb") as f:
        project = tomllib.load(f)["project"]
    assert not any(d.split("[")[0].strip() == "dbos" for d in project["dependencies"])
    assert "dbos" in project["optional-dependencies"]["dbos"]


def test_client_import_does_not_need_dbos():
    result = _run_without_dbos(
        "import client.python.trajectory_client as c\n"
        "from trajectory_ir.runtime.log import NodeLog\n"
        "assert callable(c.project)\n"
        "print('ok')\n"
    )
    assert result.returncode == 0, result.stderr
    assert "ok" in result.stdout


def test_open_trajectory_without_dbos_names_the_extra():
    result = _run_without_dbos(
        "from client.python.trajectory_client import open_trajectory\n"
        "open_trajectory('demo', 't1', db_path=':memory:')\n"
    )
    assert result.returncode != 0
    assert "trajectory-ir[dbos]" in result.stderr
