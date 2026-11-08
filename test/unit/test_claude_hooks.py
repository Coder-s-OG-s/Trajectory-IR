"""Hook processes block a bad pre-tool event and do not raise on a bad post-tool event."""

from __future__ import annotations

import json
from io import StringIO
from pathlib import Path

import pytest
from integrations.claude.config import tool_use_id
from integrations.claude.post_tool_use import main as post_main
from integrations.claude.post_tool_use_failure import main as failure_main
from integrations.claude.pre_tool_use import main as pre_main

from trajectory_ir.runtime.log import NodeLog


def test_pre_tool_rejects_bad_json(monkeypatch) -> None:
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert pre_main() == 2


def test_post_tool_ignores_bad_json(monkeypatch) -> None:
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert post_main() == 0


def test_pre_tool_seals_from_stdin(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-1")
    event = {
        "tool_name": "echo",
        "tool_use_id": "stdin-use",
        "tool_input": {"msg": "hi"},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    assert (tmp_path / "nodes.sqlite").is_file()


def test_tool_use_id_rejects_a_missing_id() -> None:
    with pytest.raises(ValueError, match="tool_use_id"):
        tool_use_id({"tool_name": "send_email"})


def test_pre_tool_rejects_a_missing_id(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-miss")
    event = {"tool_name": "send_email", "tool_input": {"to": "a"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    assert not (tmp_path / "nodes.sqlite").exists()


def test_failure_hook_records_abort_and_ignores_bad_json(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-fail")
    monkeypatch.setattr("sys.stdin", StringIO("{"))
    assert failure_main() == 0

    seal = {"tool_name": "Bash", "tool_use_id": "fail-1", "tool_input": {"command": "false"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(seal)))
    assert pre_main() == 0
    failed = {"tool_use_id": "fail-1", "error": "exit 1"}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(failed)))
    assert failure_main() == 0

    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("stdin-fail", tenant_id="demo")
    finally:
        log.close()
    abort = next(row for row in rows if row["kind"] == "ABORT")
    assert abort["payload"]["reason"] == "HOST_REPORTED_FAILURE"
    assert abort["payload"]["error"] == "exit 1"
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert "fail-1" not in state["pending"]


def test_same_owner_pending_bash_exits_2(monkeypatch, tmp_path: Path) -> None:
    """The same hook asking again before PostToolUse does not run Bash again."""
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-gate")
    monkeypatch.setenv("TRAJIR_HOOK_OWNER", "project")
    event = {
        "tool_name": "Bash",
        "tool_use_id": "bash-dup",
        "tool_input": {"command": "echo hi"},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert state["next_step"] == 2
    assert "replay" not in state["pending"]["bash-dup"]
    assert "bash-dup" not in state["done"]
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("stdin-gate", tenant_id="demo")
    finally:
        log.close()
    assert [row["kind"] for row in rows].count("DECISION") == 1


def test_other_owner_pending_bash_exits_0(monkeypatch, tmp_path: Path) -> None:
    """A second registration is not a crash retry."""
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-other")
    monkeypatch.setenv("TRAJIR_HOOK_OWNER", "project")
    event = {
        "tool_name": "Bash",
        "tool_use_id": "bash-other",
        "tool_input": {"command": "echo hi"},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    monkeypatch.setenv("TRAJIR_HOOK_OWNER", "user")
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert state["pending"]["bash-other"]["owner"] == "project"
    assert "replay" not in state["pending"]["bash-other"]


def test_pending_row_without_effect_class_blocks_the_same_owner(
    monkeypatch, tmp_path: Path
) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-empty-effect")
    event = {
        "tool_name": "Bash",
        "tool_use_id": "bash-empty",
        "tool_input": {"command": "echo hi"},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    state_path = tmp_path / "hook_state.json"
    state = json.loads(state_path.read_text(encoding="utf-8"))
    del state["pending"]["bash-empty"]["effect_class"]
    state_path.write_text(json.dumps(state), encoding="utf-8")
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    monkeypatch.setenv("TRAJIR_HOOK_OWNER", "user")
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0


def test_recorded_row_without_effect_class_exits_2(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-done-empty")
    event = {
        "tool_name": "echo",
        "tool_use_id": "done-empty",
        "tool_input": {"msg": "hi"},
        "annotations": {"readOnlyHint": True, "destructiveHint": False},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    done = {"tool_use_id": "done-empty", "tool_response": "hi"}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(done)))
    assert post_main() == 0
    state_path = tmp_path / "hook_state.json"
    state = json.loads(state_path.read_text(encoding="utf-8"))
    del state["done"]["done-empty"]["effect_class"]
    state_path.write_text(json.dumps(state), encoding="utf-8")
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2


def test_recorded_gated_pre_tool_exits_2(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-gate")
    event = {
        "tool_name": "Bash",
        "tool_use_id": "bash-replay",
        "tool_input": {"command": "echo hi"},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    done = {"tool_use_id": "bash-replay", "tool_response": {"ok": True}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(done)))
    assert post_main() == 0
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert "bash-replay" not in state["pending"]
    assert state["done"]["bash-replay"]["recorded"] is True
    log = NodeLog(str(tmp_path / "nodes.sqlite"))
    try:
        rows = log.list_nodes("stdin-gate", tenant_id="demo")
    finally:
        log.close()
    assert [row["kind"] for row in rows].count("DECISION") == 1
    assert [row["kind"] for row in rows].count("TOOL_RESULT") == 1


def test_read_only_replay_still_exits_0(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-ro")
    event = {
        "tool_name": "lookup",
        "tool_use_id": "ro-1",
        "tool_input": {"q": "x"},
        "annotations": {"readOnlyHint": True, "destructiveHint": False},
    }
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    state = json.loads((tmp_path / "hook_state.json").read_text(encoding="utf-8"))
    assert state["pending"]["ro-1"]["effect_class"] == "READ_ONLY"
    assert "replay" not in state["pending"]["ro-1"]


def test_missing_identity_file_names_the_installer(monkeypatch, tmp_path: Path, capsys) -> None:
    missing = tmp_path / "missing.json"
    monkeypatch.setenv("TRAJIR_IDENTITY_FILE", str(missing))
    event = {"tool_name": "Bash", "tool_use_id": "id-miss", "tool_input": {"command": "true"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    err = capsys.readouterr().err
    assert "install_claude" in err
    assert "missing.json" in err
    assert not (tmp_path / "nodes.sqlite").exists()


def test_identity_file_picks_the_work_dir(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.delenv("TRAJIR_WORK_DIR", raising=False)
    monkeypatch.delenv("TRAJIR_TENANT_ID", raising=False)
    monkeypatch.delenv("TRAJIR_TRAJECTORY_ID", raising=False)
    work = tmp_path / "from-file"
    ident = tmp_path / "id.json"
    ident.write_text(
        json.dumps(
            {
                "work_dir": str(work),
                "tenant_id": "file-tenant",
                "trajectory_id": "file-traj",
            }
        ),
        encoding="utf-8",
    )
    monkeypatch.setenv("TRAJIR_IDENTITY_FILE", str(ident))
    event = {"tool_name": "echo", "tool_use_id": "id-1", "tool_input": {"msg": "hi"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 0
    assert (work / "nodes.sqlite").is_file()


def test_cwd_identity_file_does_not_redirect(monkeypatch, tmp_path: Path) -> None:
    from integrations.claude.config import hook_identity

    monkeypatch.delenv("TRAJIR_IDENTITY_FILE", raising=False)
    monkeypatch.delenv("TRAJIR_WORK_DIR", raising=False)
    monkeypatch.delenv("TRAJIR_TENANT_ID", raising=False)
    monkeypatch.delenv("TRAJIR_TRAJECTORY_ID", raising=False)
    planted = tmp_path / "planted"
    folder = tmp_path / ".local-evidence"
    folder.mkdir()
    (folder / "claude-one-step.json").write_text(
        json.dumps({"work_dir": str(planted), "tenant_id": "evil", "trajectory_id": "evil"}),
        encoding="utf-8",
    )
    monkeypatch.chdir(tmp_path)
    ident = hook_identity()
    assert ident["tenant_id"] == "local"
    assert ident["trajectory_id"] == "claude-one-step"
    assert "planted" not in ident["work_dir"]


def test_bad_identity_json_exits_2(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.delenv("TRAJIR_WORK_DIR", raising=False)
    ident = tmp_path / "id.json"
    ident.write_text("{", encoding="utf-8")
    monkeypatch.setenv("TRAJIR_IDENTITY_FILE", str(ident))
    event = {"tool_name": "echo", "tool_use_id": "bad-json", "tool_input": {"msg": "hi"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2


def test_missing_identity_file_exits_2(monkeypatch, tmp_path: Path) -> None:
    monkeypatch.delenv("TRAJIR_WORK_DIR", raising=False)
    monkeypatch.setenv("TRAJIR_IDENTITY_FILE", str(tmp_path / "missing.json"))
    event = {"tool_name": "echo", "tool_use_id": "miss-file", "tool_input": {"msg": "hi"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(event)))
    assert pre_main() == 2
    assert not (tmp_path / "nodes.sqlite").exists()


def test_post_tool_record_failure_still_exits_zero(monkeypatch, tmp_path: Path) -> None:
    """The tool already ran. A diary write failure must not ask the host to run it again."""
    monkeypatch.setenv("TRAJIR_WORK_DIR", str(tmp_path))
    monkeypatch.setenv("TRAJIR_TENANT_ID", "demo")
    monkeypatch.setenv("TRAJIR_TRAJECTORY_ID", "stdin-record-fail")
    seal = {"tool_name": "echo", "tool_use_id": "rec-1", "tool_input": {"msg": "hi"}}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(seal)))
    assert pre_main() == 0

    def boom(*_args: object, **_kwargs: object) -> dict:
        raise RuntimeError("sqlite locked")

    monkeypatch.setattr("integrations.claude.post_tool_use.record_tool_result", boom)
    done = {"tool_use_id": "rec-1", "tool_response": "hi"}
    monkeypatch.setattr("sys.stdin", StringIO(json.dumps(done)))
    assert post_main() == 0
