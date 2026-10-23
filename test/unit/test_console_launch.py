"""Console start stays on 127.0.0.1:8787 and does not replace a listener."""

from __future__ import annotations

import os
from pathlib import Path

import pytest
from integrations.attach.console_launch import (
    console_is_up,
    start_console_if_free,
)


def test_other_addresses_are_refused() -> None:
    with pytest.raises(ValueError, match=r"127\.0\.0\.1:8787"):
        console_is_up("127.0.0.1:9")
    with pytest.raises(ValueError, match=r"127\.0\.0\.1:8787"):
        console_is_up("0.0.0.0:8787")


def test_probe_does_not_bind() -> None:
    assert console_is_up() in (True, False)


def test_already_up_does_not_spawn(tmp_path: Path) -> None:
    def popen(*_args: object, **_kwargs: object) -> object:
        raise AssertionError("spawned while the port was taken")

    result = start_console_if_free(tmp_path, probe=lambda: True, popen=popen)
    assert result.status == "already"
    assert result.process is None
    assert not (tmp_path / ".console-data").exists()


def test_start_command_is_only_8787(tmp_path: Path) -> None:
    exe = tmp_path / "go" / "trajir-console.exe"
    exe.parent.mkdir(parents=True)
    exe.write_bytes(b"")
    seen: dict[str, object] = {}

    class _Proc:
        pid = 7

    proc = _Proc()

    def popen(cmd: list[str], **kwargs: object) -> _Proc:
        seen["cmd"] = list(cmd)
        seen["kwargs"] = kwargs
        return proc

    data = tmp_path / "data"
    result = start_console_if_free(
        tmp_path,
        data_dir=data,
        detach=False,
        probe=lambda: False,
        popen=popen,
        confirm=False,
    )
    assert result.status == "started"
    assert result.process is proc
    cmd = seen["cmd"]
    assert isinstance(cmd, list)
    assert cmd[0] == str(exe)
    assert cmd[1:3] == ["-addr", "127.0.0.1:8787"]
    assert str(data) in cmd
    kwargs = seen["kwargs"]
    assert isinstance(kwargs, dict)
    assert "creationflags" not in kwargs
    assert "start_new_session" not in kwargs


def test_default_data_dir_is_under_local_evidence(tmp_path: Path) -> None:
    exe = tmp_path / "go" / "trajir-console.exe"
    exe.parent.mkdir(parents=True)
    exe.write_bytes(b"")
    seen: dict[str, object] = {}

    def popen(cmd: list[str], **_kwargs: object) -> object:
        seen["cmd"] = list(cmd)
        return object()

    result = start_console_if_free(tmp_path, probe=lambda: False, popen=popen, confirm=False)
    assert result.status == "started"
    data = tmp_path / ".local-evidence" / "console-data"
    assert data.is_dir()
    cmd = seen["cmd"]
    assert isinstance(cmd, list)
    assert str(data) in cmd
    assert not (tmp_path / ".console-data").exists()


def test_confirm_waits_until_the_probe_accepts(tmp_path: Path) -> None:
    exe = tmp_path / "go" / "trajir-console.exe"
    exe.parent.mkdir(parents=True)
    exe.write_bytes(b"")
    calls = {"n": 0}

    def probe() -> bool:
        calls["n"] += 1
        return calls["n"] >= 2

    class _Proc:
        def poll(self) -> None:
            return None

    result = start_console_if_free(
        tmp_path,
        data_dir=tmp_path / "data",
        probe=probe,
        popen=lambda *_a, **_k: _Proc(),
        confirm=True,
        wait_s=1.0,
    )
    assert result.status == "started"
    assert calls["n"] >= 2


def test_confirm_reports_down_when_the_process_exits(tmp_path: Path) -> None:
    exe = tmp_path / "go" / "trajir-console.exe"
    exe.parent.mkdir(parents=True)
    exe.write_bytes(b"")

    class _Proc:
        def poll(self) -> int:
            return 1

    result = start_console_if_free(
        tmp_path,
        data_dir=tmp_path / "data",
        probe=lambda: False,
        popen=lambda *_a, **_k: _Proc(),
        confirm=True,
        wait_s=1.0,
    )
    assert result.status == "down"


def test_detach_sets_a_new_session(tmp_path: Path) -> None:
    exe = tmp_path / "go" / "trajir-console.exe"
    exe.parent.mkdir(parents=True)
    exe.write_bytes(b"")
    seen: dict[str, object] = {}

    def popen(_cmd: list[str], **kwargs: object) -> object:
        seen["kwargs"] = kwargs
        return object()

    start_console_if_free(tmp_path, detach=True, probe=lambda: False, popen=popen, confirm=False)
    kwargs = seen["kwargs"]
    assert isinstance(kwargs, dict)
    if os.name == "nt":
        assert int(kwargs["creationflags"]) != 0
    else:
        assert kwargs["start_new_session"] is True
