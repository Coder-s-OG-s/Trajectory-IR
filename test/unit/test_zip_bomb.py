import zipfile
from pathlib import Path

import pytest

from trajectory_ir.package.tir import (
    TirLimitError,
    _read_zip_member,
)


def test_read_zip_member_enforces_budget(tmp_path: Path):
    zip_path = tmp_path / "bomb.zip"
    with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        # Create a file that is exactly 100 bytes long
        zf.writestr("bomb.txt", b"x" * 100)

    # Budget is 50 bytes. We expect reading this member to fail.
    budget = [50]

    with (
        zipfile.ZipFile(zip_path, "r") as zf,
        pytest.raises(TirLimitError, match="zip total uncompressed size would exceed limit"),
    ):
        _read_zip_member(zf, "bomb.txt", budget=budget)


def test_read_zip_member_enforces_entry_limit(tmp_path: Path, monkeypatch):
    import trajectory_ir.package.tir as tir

    monkeypatch.setattr(tir, "MAX_UNCOMPRESSED_ENTRY_BYTES", 50)

    zip_path = tmp_path / "bomb.zip"
    with zipfile.ZipFile(zip_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("bomb.txt", b"x" * 100)

    budget = [1000]  # large enough
    with (
        zipfile.ZipFile(zip_path, "r") as zf,
        pytest.raises(TirLimitError, match="exceeds limit"),
    ):
        _read_zip_member(zf, "bomb.txt", budget=budget)
