"""Smoke tests for examples/host_loop (public client only)."""

from __future__ import annotations

import os
import sys

import pytest

_REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_HOST = os.path.join(_REPO, "examples", "host_loop")
if _HOST not in sys.path:
    sys.path.insert(0, _HOST)

import run_host  # noqa: E402


def test_live_host_step(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "host.sqlite")
    results = run_host.run_one_step(db_path=db, sandbox=False)
    assert results == ["hello, trajectory", "deployed:1.0.0"]


def test_sandbox_rejects_deploy(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "host-sandbox.sqlite")
    with pytest.raises(run_host.SandboxForbidden):
        run_host.run_one_step(db_path=db, sandbox=True)


def test_main_live_exit_zero(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    code = run_host.main(["--db", str(tmp_path / "m.sqlite")])
    assert code == 0


def test_main_sandbox_exit_zero(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    code = run_host.main(["--sandbox", "--db", str(tmp_path / "s.sqlite")])
    assert code == 0
