"""The hook launcher fails closed on pre and stays quiet on post."""

from __future__ import annotations

import json
import os
import subprocess
import sys
from pathlib import Path

from integrations.attach.launch_hook import main as launch_main

_ROOT = Path(__file__).resolve().parents[2]
_PRE = _ROOT / "integrations" / "claude" / "pre_tool_use.py"
_LAUNCH = _ROOT / "integrations" / "attach" / "launch_hook.py"


def _clean_env() -> dict[str, str]:
    env = dict(os.environ)
    for key in (
        "TRAJIR_WORK_DIR",
        "TRAJIR_TENANT_ID",
        "TRAJIR_TRAJECTORY_ID",
        "TRAJIR_IDENTITY_FILE",
        "TRAJIR_HOOK_OWNER",
    ):
        env.pop(key, None)
    return env


def test_missing_script_exits_2_on_pre_and_0_on_post(tmp_path: Path) -> None:
    ident = tmp_path / "id.json"
    ident.write_text("{}\n", encoding="utf-8")
    missing = tmp_path / "missing.py"
    assert launch_main([str(ident), str(missing), "pre"]) == 2
    assert launch_main([str(ident), str(missing), "post"]) == 0
    assert launch_main(["only-one"]) == 2


def test_script_outside_the_install_is_not_executed(tmp_path: Path) -> None:
    ident = tmp_path / "id.json"
    ident.write_text("{}\n", encoding="utf-8")
    marker = tmp_path / "ran.txt"
    rogue = tmp_path / "rogue.py"
    rogue.write_text(
        f"from pathlib import Path\nPath({str(marker)!r}).write_text('ran', encoding='utf-8')\n",
        encoding="utf-8",
    )
    assert launch_main([str(ident), str(rogue), "pre"]) == 2
    assert launch_main([str(ident), str(rogue), "post"]) == 0
    assert not marker.exists()


def test_import_error_on_pre_exits_2(tmp_path: Path, monkeypatch) -> None:
    ident = tmp_path / "id.json"
    ident.write_text("{}\n", encoding="utf-8")
    broken = tmp_path / "broken.py"
    broken.write_text("import this_module_does_not_exist\n", encoding="utf-8")
    monkeypatch.setattr(
        "integrations.attach.launch_hook._allowed_script",
        lambda _script: broken.resolve(),
    )
    assert launch_main([str(ident), str(broken), "pre"]) == 2
    assert launch_main([str(ident), str(broken), "post"]) == 0
    assert not (tmp_path / "work").exists()


def test_subprocess_other_owner_bash_exits_0(tmp_path: Path) -> None:
    work = tmp_path / "work"
    ident = tmp_path / "id.json"
    ident.write_text(
        json.dumps(
            {
                "work_dir": str(work),
                "tenant_id": "local",
                "trajectory_id": "launch-owner",
            }
        ),
        encoding="utf-8",
    )
    event = json.dumps(
        {
            "tool_name": "Bash",
            "tool_use_id": "bash-owner",
            "tool_input": {"command": "echo hi"},
        }
    )
    env = _clean_env()
    first_cmd = [sys.executable, str(_LAUNCH), str(ident), str(_PRE), "pre", "project"]
    second_cmd = [sys.executable, str(_LAUNCH), str(ident), str(_PRE), "pre", "user"]
    first = subprocess.run(
        first_cmd, input=event, text=True, capture_output=True, env=env, check=False
    )
    assert first.returncode == 0, first.stderr
    second = subprocess.run(
        second_cmd, input=event, text=True, capture_output=True, env=env, check=False
    )
    assert second.returncode == 0, second.stderr


def test_subprocess_duplicate_bash_exits_2_before_record(tmp_path: Path) -> None:
    work = tmp_path / "work"
    ident = tmp_path / "id.json"
    ident.write_text(
        json.dumps(
            {
                "work_dir": str(work),
                "tenant_id": "local",
                "trajectory_id": "launch-bash",
            }
        ),
        encoding="utf-8",
    )
    event = json.dumps(
        {
            "tool_name": "Bash",
            "tool_use_id": "bash-sub",
            "tool_input": {"command": "echo hi"},
        }
    )
    env = _clean_env()
    cmd = [sys.executable, str(_LAUNCH), str(ident), str(_PRE), "pre"]
    first = subprocess.run(cmd, input=event, text=True, capture_output=True, env=env, check=False)
    assert first.returncode == 0, first.stderr
    second = subprocess.run(cmd, input=event, text=True, capture_output=True, env=env, check=False)
    assert second.returncode == 2, second.stderr
    state = json.loads((work / "hook_state.json").read_text(encoding="utf-8"))
    assert state["next_step"] == 2
    assert "replay" not in state["pending"]["bash-sub"]

    post = _ROOT / "integrations" / "claude" / "post_tool_use.py"
    recorded = json.dumps({"tool_use_id": "bash-sub", "tool_response": {"ok": True}})
    post_cmd = [sys.executable, str(_LAUNCH), str(ident), str(post), "post"]
    wrote = subprocess.run(
        post_cmd, input=recorded, text=True, capture_output=True, env=env, check=False
    )
    assert wrote.returncode == 0, wrote.stderr
    third = subprocess.run(cmd, input=event, text=True, capture_output=True, env=env, check=False)
    assert third.returncode == 2, third.stderr
