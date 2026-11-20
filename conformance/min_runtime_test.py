"""A package that asks for a newer runtime must not load.

Issue #563. Writers still store min_runtime 0.1.0, which this reader accepts.
A higher value fails in the conformance suite the same way the unit tests do.
"""

from __future__ import annotations

import json
import zipfile
from pathlib import Path

import pytest

from trajectory_ir.package import RUNTIME_VERSION, TirRuntimeError, export_tir, load_tir
from trajectory_ir.runtime.log import NodeLog


def _set_min_runtime(src: Path, dst: Path, value: str) -> Path:
    with zipfile.ZipFile(src) as zin, zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zout:
        for item in zin.infolist():
            data = zin.read(item.filename)
            if item.filename == "COMPAT.json":
                doc = json.loads(data)
                doc["min_runtime"] = value
                data = (json.dumps(doc, indent=2, sort_keys=True) + "\n").encode()
            zout.writestr(item, data)
    return dst


def test_current_package_loads_and_a_newer_runtime_does_not(tmp_path: Path) -> None:
    log = NodeLog(str(tmp_path / "src.sqlite"))
    try:
        log.append("PROJECT_CONTEXT", 1, {"goal": "runtime"}, "t-rt", "demo", seq=0)
        log.append("DECISION", 1, {"plan": {"tool_calls": []}}, "t-rt", "demo", seq=1)
        pkg = tmp_path / "run.tir"
        export_tir(log, "t-rt", pkg, mode="thin")
    finally:
        log.close()

    assert len(load_tir(pkg).nodes) == 2

    major, minor, patch = (int(part) for part in RUNTIME_VERSION.split("."))
    newer = f"{major}.{minor}.{patch + 1}"
    future = _set_min_runtime(pkg, tmp_path / "future.tir", newer)
    with pytest.raises(TirRuntimeError) as info:
        load_tir(future)
    assert newer in str(info.value)
    assert RUNTIME_VERSION in str(info.value)
