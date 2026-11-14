"""package_format checks on load (MASTER_SPECIFICATION section 9)."""

from __future__ import annotations

import json
import zipfile
from pathlib import Path

import pytest

from trajectory_ir.package import (
    PACKAGE_FORMAT_VERSION,
    TirError,
    TirFormatError,
    export_tir,
    load_tir,
    load_tir_unverified,
)
from trajectory_ir.runtime.log import NodeLog


@pytest.fixture
def package(tmp_path: Path) -> Path:
    log = NodeLog(str(tmp_path / "src.sqlite"))
    try:
        log.append("PROJECT_CONTEXT", 1, {"goal": "g"}, "t-fmt", "demo", seq=0)
        log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t-fmt", "demo", seq=1)
        out = tmp_path / "run.tir"
        export_tir(log, "t-fmt", out, mode="thin")
    finally:
        log.close()
    return out


def _rewrite(src: Path, dst: Path, manifest_format: str | None, compat_format: str | None) -> Path:
    """Copy src, setting package_format in manifest.json and COMPAT.json. None deletes it."""

    def patch(raw: bytes, value: str | None) -> bytes:
        doc = json.loads(raw)
        if value is None:
            doc.pop("package_format", None)
        else:
            doc["package_format"] = value
        return (json.dumps(doc, indent=2, sort_keys=True) + "\n").encode()

    with zipfile.ZipFile(src) as zin, zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zout:
        for item in zin.infolist():
            data = zin.read(item.filename)
            if item.filename == "manifest.json":
                data = patch(data, manifest_format)
            elif item.filename == "COMPAT.json":
                data = patch(data, compat_format)
            zout.writestr(item, data)
    return dst


def test_current_format_loads(package: Path) -> None:
    assert len(load_tir(package).nodes) == 2


def test_unknown_major_is_rejected(package: Path, tmp_path: Path, monkeypatch) -> None:
    bad = _rewrite(package, tmp_path / "future.tir", "1.0", "1.0")
    with pytest.raises(TirFormatError) as info:
        load_tir(bad)
    assert "1.0" in str(info.value)
    assert PACKAGE_FORMAT_VERSION in str(info.value)
    assert isinstance(info.value, TirError)
    monkeypatch.setenv("TRAJIR_ALLOW_UNVERIFIED", "1")
    with pytest.raises(TirFormatError):
        load_tir_unverified(bad)


def test_newer_minor_loads(package: Path, tmp_path: Path) -> None:
    newer = _rewrite(package, tmp_path / "minor.tir", "0.9", "0.9")
    assert len(load_tir(newer).nodes) == 2


@pytest.mark.parametrize(
    ("manifest_format", "compat_format"),
    [
        ("0.1", "0.2"),
        ("0.1", None),
        (None, "0.1"),
        ("0.1.0", "0.1.0"),
        ("v0.1", "v0.1"),
    ],
    ids=["disagree", "missing-compat", "missing-manifest", "three-parts", "not-numeric"],
)
def test_bad_format_fields_are_rejected(
    package: Path, tmp_path: Path, manifest_format: str | None, compat_format: str | None
) -> None:
    bad = _rewrite(package, tmp_path / "bad.tir", manifest_format, compat_format)
    with pytest.raises(TirFormatError):
        load_tir(bad)
