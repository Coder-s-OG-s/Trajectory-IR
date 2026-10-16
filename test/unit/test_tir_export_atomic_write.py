"""export_tir must never leave a partially written .tir file at dest_path."""

from __future__ import annotations

from pathlib import Path

import pytest

from trajectory_ir.package import export_tir, load_tir
from trajectory_ir.runtime.log import NodeLog


@pytest.fixture
def sample_log(tmp_path: Path) -> NodeLog:
    src_dir = tmp_path / "src"
    src_dir.mkdir()
    log = NodeLog(str(src_dir / "src.sqlite"))
    log.append(
        "PROJECT_CONTEXT",
        1,
        {"goal": "demo"},
        trajectory_id="t-atomic",
        tenant_id="demo",
        seq=0,
    )
    log.append(
        "DECISION",
        1,
        {"plan": {"tool_calls": []}},
        trajectory_id="t-atomic",
        tenant_id="demo",
        seq=1,
    )
    return log


def test_export_writes_no_temp_file_left_behind(tmp_path, sample_log):
    export_dir = tmp_path / "export"
    export_dir.mkdir()
    dest = export_dir / "run.tir"
    export_tir(sample_log, "t-atomic", dest, mode="fat", tenant_id="demo")

    assert dest.is_file()
    leftovers = [p for p in dest.parent.iterdir() if p != dest]
    assert leftovers == []


def test_export_does_not_touch_existing_dest_on_failure(tmp_path, sample_log, monkeypatch):
    export_dir = tmp_path / "export"
    export_dir.mkdir()
    dest = export_dir / "run.tir"
    export_tir(sample_log, "t-atomic", dest, mode="fat", tenant_id="demo")
    original_bytes = dest.read_bytes()

    import zipfile

    def boom(*args, **kwargs):
        raise RuntimeError("simulated crash mid export")

    monkeypatch.setattr(zipfile.ZipFile, "writestr", boom)

    with pytest.raises(RuntimeError):
        export_tir(sample_log, "t-atomic", dest, mode="fat", tenant_id="demo")

    # dest_path must still hold the last good export, not a truncated file,
    # and no stray temp file should remain next to it.
    assert dest.read_bytes() == original_bytes
    leftovers = [p for p in dest.parent.iterdir() if p != dest]
    assert leftovers == []


def test_exported_package_still_loads(tmp_path, sample_log):
    dest = tmp_path / "run.tir"
    export_tir(sample_log, "t-atomic", dest, mode="fat", tenant_id="demo")
    pkg = load_tir(dest)
    assert pkg.manifest["mode"] == "fat"


def test_export_sign_failure_keeps_existing_package(tmp_path, sample_log, monkeypatch):
    export_dir = tmp_path / "export"
    export_dir.mkdir()
    dest = export_dir / "run.tir"
    export_tir(sample_log, "t-atomic", dest, mode="thin", tenant_id="demo")
    original = dest.read_bytes()

    def boom(*args, **kwargs):
        raise RuntimeError("simulated sign failure")

    monkeypatch.setattr("trajectory_ir.package.tir.sign_package", boom)

    notices = []
    key = b"\x00" * 64
    with pytest.raises(RuntimeError, match="simulated sign failure"):
        export_tir(
            sample_log,
            "t-atomic",
            dest,
            mode="thin",
            tenant_id="demo",
            sign_key=key,
            on_exported=notices.append,
        )

    assert dest.read_bytes() == original
    leftovers = [p for p in dest.parent.iterdir() if p != dest]
    assert leftovers == []
    assert len(notices) == 1
    assert notices[0]["ok"] is False
    assert notices[0]["error"] == "RuntimeError"


def test_export_sign_failure_without_existing_leaves_nothing(tmp_path, sample_log, monkeypatch):
    dest = tmp_path / "fresh.tir"

    def boom(*args, **kwargs):
        raise RuntimeError("simulated sign failure")

    monkeypatch.setattr("trajectory_ir.package.tir.sign_package", boom)

    key = b"\x00" * 64
    with pytest.raises(RuntimeError, match="simulated sign failure"):
        export_tir(sample_log, "t-atomic", dest, mode="thin", tenant_id="demo", sign_key=key)

    assert not dest.exists()
    leftovers = [p for p in dest.parent.iterdir() if p.is_file()]
    assert leftovers == []
