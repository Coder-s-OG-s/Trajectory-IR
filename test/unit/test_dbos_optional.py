import os
import subprocess
import sys
import tomllib
from pathlib import Path

import pytest

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


def test_open_trajectory_and_resume_work_without_dbos(tmp_path):
    # The QUICKSTART section 2 flow, then resume on the same log, on a base install.
    db = str(tmp_path / "qs.sqlite")
    result = _run_without_dbos(
        "from client.python.trajectory_client import (\n"
        "    commit_step, exec_tool, open_trajectory, project, resume, seal_decision,\n"
        ")\n"
        "from trajectory_ir.effects import EffectClass\n"
        "from trajectory_ir.runtime.tool import Tool\n"
        f"db = {db!r}\n"
        "traj = open_trajectory(tenant_id='demo', trajectory_id='qs-1', db_path=db)\n"
        "project(traj, step_n=1, context={'goal': 'hello'})\n"
        "seal_decision(traj, step_n=1, plan={'tool_calls': [{'name': 'echo', 'args': {}}]})\n"
        "tool = Tool(name='echo', fn=lambda msg: msg, effect_class=EffectClass.PURE)\n"
        "out = exec_tool(traj, step_n=1, call={'args': {'msg': 'hi'}}, tool=tool, seq=2)\n"
        "assert out.result == 'hi'\n"
        "commit_step(traj, step_n=1, seq=4)\n"
        "traj.close()\n"
        "again = resume('qs-1', tenant_id='demo', db_path=db)\n"
        "assert again.trajectory_id == 'qs-1'\n"
        "again.close()\n"
        "print('ok')\n"
    )
    assert result.returncode == 0, result.stderr
    assert "ok" in result.stdout


@pytest.mark.parametrize(
    "call",
    [
        "open_trajectory('demo', 't1', db_path=db, durable=True)",
        "resume('t1', tenant_id='demo', db_path=db, durable=True)",
    ],
)
def test_durable_true_without_dbos_names_the_extra(tmp_path, call):
    db = str(tmp_path / "t.sqlite")
    result = _run_without_dbos(
        "from client.python.trajectory_client import open_trajectory, resume\n"
        f"db = {db!r}\n"
        f"{call}\n"
    )
    assert result.returncode != 0
    assert "trajectory-ir[dbos]" in result.stderr


def test_make_run_step_with_injected_hooks_does_not_need_dbos(tmp_path):
    db = str(tmp_path / "nodes.sqlite")
    result = _run_without_dbos(
        "from drivers.durable_backend.restate import (\n"
        "    durable_infer, durable_tool, durable_workflow, init_backend,\n"
        ")\n"
        "from drivers.durable_backend.restate.local_memo import clear_memo, workflow_scope\n"
        "from trajectory_ir.effects import EffectClass\n"
        "from trajectory_ir.resume.step import make_run_step\n"
        "from trajectory_ir.runtime.log import NodeLog\n"
        "from trajectory_ir.runtime.tool import Tool\n"
        "clear_memo()\n"
        "init_backend()\n"
        f"log = NodeLog({db!r})\n"
        "tools = {'echo': Tool(name='echo', fn=lambda *, msg: msg,"
        " effect_class=EffectClass.PURE)}\n"
        "run_step = make_run_step(\n"
        "    log, 'demo', 'rs-1', tools,\n"
        "    durable_infer_fn=durable_infer,\n"
        "    durable_tool_fn=durable_tool,\n"
        "    durable_workflow_fn=durable_workflow,\n"
        ")\n"
        "plan = {'tool_calls': [{'name': 'echo', 'args': {'msg': 'hi'}}]}\n"
        "with workflow_scope('rs-1'):\n"
        "    assert run_step(step_n=1, model_call=lambda ctx: plan, context={}) == ['hi']\n"
        "assert 'dbos' not in [m for m in sys.modules if sys.modules[m] is not None]\n"
        "print('ok')\n"
    )
    assert result.returncode == 0, result.stderr
    assert "ok" in result.stdout


def test_durable_flag_controls_dbos_launch(monkeypatch, tmp_path):
    import client.python.trajectory_client as c
    from drivers.durable_backend.dbos import adapter

    launched = []
    monkeypatch.setattr(adapter, "init_backend", lambda db_path: launched.append(db_path))
    db = str(tmp_path / "t.sqlite")

    c.open_trajectory("demo", "t1", db_path=db, durable=False).close()
    assert launched == []
    # Default keeps the old behaviour when the extra is installed.
    c.open_trajectory("demo", "t1", db_path=db).close()
    assert launched == [db]
