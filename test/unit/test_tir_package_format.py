"""package_format checks on load (MASTER_SPECIFICATION section 9)."""

from __future__ import annotations

import json
import tomllib
import zipfile
from pathlib import Path

import pytest

from trajectory_ir.package import (
    PACKAGE_FORMAT_VERSION,
    RUNTIME_VERSION,
    TirError,
    TirFormatError,
    TirRuntimeError,
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


def _set_min_runtime(src: Path, dst: Path, value: str | None) -> Path:
    """Copy src, setting COMPAT.json min_runtime. None deletes the key."""

    def patch(raw: bytes) -> bytes:
        doc = json.loads(raw)
        if value is None:
            doc.pop("min_runtime", None)
        else:
            doc["min_runtime"] = value
        return (json.dumps(doc, indent=2, sort_keys=True) + "\n").encode()

    with zipfile.ZipFile(src) as zin, zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zout:
        for item in zin.infolist():
            data = zin.read(item.filename)
            if item.filename == "COMPAT.json":
                data = patch(data)
            zout.writestr(item, data)
    return dst


def test_runtime_version_matches_pyproject() -> None:
    path = Path(__file__).resolve().parents[2] / "pyproject.toml"
    with path.open("rb") as handle:
        doc = tomllib.load(handle)
    assert doc["project"]["version"] == RUNTIME_VERSION


@pytest.mark.parametrize("value", ["99.0.0", "0.4.1", "0.10.0"])
def test_newer_min_runtime_is_rejected(
    package: Path, tmp_path: Path, value: str, monkeypatch
) -> None:
    bad = _set_min_runtime(package, tmp_path / "future.tir", value)
    with pytest.raises(TirRuntimeError) as info:
        load_tir(bad)
    assert value in str(info.value)
    assert RUNTIME_VERSION in str(info.value)
    assert not isinstance(info.value, TirFormatError)
    monkeypatch.setenv("TRAJIR_ALLOW_UNVERIFIED", "1")
    with pytest.raises(TirRuntimeError):
        load_tir_unverified(bad)


@pytest.mark.parametrize("value", [RUNTIME_VERSION, "0.3.1", "0.2.10", None])
def test_min_runtime_at_or_below_reader_loads(
    package: Path, tmp_path: Path, value: str | None
) -> None:
    pkg = _set_min_runtime(package, tmp_path / "ok.tir", value)
    assert len(load_tir(pkg).nodes) == 2


@pytest.mark.parametrize("value", ["9.9", "v0.3.0", "0.3.0 "])
def test_bad_min_runtime_is_rejected(package: Path, tmp_path: Path, value: str) -> None:
    bad = _set_min_runtime(package, tmp_path / "bad-runtime.tir", value)
    with pytest.raises(TirRuntimeError) as info:
        load_tir(bad)
    assert RUNTIME_VERSION in str(info.value)
