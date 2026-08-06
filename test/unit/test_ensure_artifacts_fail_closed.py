"""ensure_artifacts_in_cas must fail closed on a manifest entry missing content_hash."""

from __future__ import annotations

import pytest

from trajectory_ir.storage import CASError, FileSystemCAS, ensure_artifacts_in_cas


def test_missing_content_hash_raises_cas_error(tmp_path):
    cas = FileSystemCAS(tmp_path / "cas")
    manifest = [{"logical_path": "out.bin"}]  # no content_hash key

    with pytest.raises(CASError):
        ensure_artifacts_in_cas(cas, manifest)


def test_null_content_hash_raises_cas_error(tmp_path):
    cas = FileSystemCAS(tmp_path / "cas")
    manifest = [{"logical_path": "out.bin", "content_hash": None}]

    with pytest.raises(CASError):
        ensure_artifacts_in_cas(cas, manifest)


def test_missing_content_hash_raises_even_when_require_all_false(tmp_path):
    cas = FileSystemCAS(tmp_path / "cas")
    manifest = [{"logical_path": "out.bin"}]

    with pytest.raises(CASError):
        ensure_artifacts_in_cas(cas, manifest, require_all=False)


def test_present_content_hash_passes(tmp_path):
    cas = FileSystemCAS(tmp_path / "cas")
    h = cas.put(b"payload")
    manifest = [{"logical_path": "out.bin", "content_hash": h}]

    ensure_artifacts_in_cas(cas, manifest)
