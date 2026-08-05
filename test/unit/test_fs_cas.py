"""Unit tests for local filesystem CAS and thin rehydrate."""

from __future__ import annotations

import os

import pytest

from trajectory_ir.storage import (
    CASIntegrityError,
    CASNotFoundError,
    FileSystemCAS,
    content_hash,
    rehydrate_artifacts,
    shard_key,
)
from trajectory_ir.storage.cas import CASError, normalize_content_hash


@pytest.fixture
def cas_root(tmp_path):
    return tmp_path / "cas_root"


@pytest.fixture
def store(cas_root):
    return FileSystemCAS(cas_root)


def test_content_hash_known_empty():
    # SHA-256 of empty string (public test vector).
    assert content_hash(b"") == ("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")


def test_shard_key_layout():
    h = content_hash(b"hello")
    assert shard_key(h) == f"cas/{h[:2]}/{h}"


def test_put_get_roundtrip(store):
    data = b"trajectory artifact payload"
    h = store.put(data)
    assert h == content_hash(data)
    assert store.has(h)
    assert store.get(h) == data
    assert store.path_for(h).is_file()
    assert store.path_for(h) == store.root / f"cas/{h[:2]}/{h}"


def test_put_is_idempotent(store):
    data = b"same bytes twice"
    h1 = store.put(data)
    h2 = store.put(data)
    assert h1 == h2
    assert store.get(h1) == data


def test_get_missing_raises(store):
    h = content_hash(b"absent")
    with pytest.raises(CASNotFoundError):
        store.get(h)
    assert not store.has(h)


def test_get_detects_tamper(store):
    data = b"honest bytes"
    h = store.put(data)
    path = store.path_for(h)
    path.write_bytes(b"tampered")
    with pytest.raises(CASIntegrityError):
        store.get(h)


def test_normalize_content_hash_rejects_bad():
    with pytest.raises(CASError):
        normalize_content_hash("not-a-hash")
    with pytest.raises(CASError):
        normalize_content_hash("ab")


def test_uri_for_is_portable_hash_only(store):
    h = store.put(b"x")
    assert store.uri_for(h) == f"cas://{h}"


def test_rehydrate_artifacts_require_all(store):
    h1 = store.put(b"one")
    h2 = content_hash(b"two-missing")
    manifest = [
        {"logical_path": "a.bin", "content_hash": h1},
        {"logical_path": "b.bin", "content_hash": h2},
    ]
    with pytest.raises(CASNotFoundError):
        rehydrate_artifacts(store, manifest, require_all=True)

    partial = rehydrate_artifacts(store, manifest, require_all=False)
    assert list(partial.keys()) == [h1]
    assert partial[h1] == b"one"


def test_rehydrate_full(store):
    h1 = store.put(b"alpha")
    h2 = store.put(b"beta")
    got = rehydrate_artifacts(
        store,
        [
            {"content_hash": h1, "logical_path": "a"},
            {"content_hash": h2, "logical_path": "b"},
        ],
    )
    assert got[h1] == b"alpha"
    assert got[h2] == b"beta"


def test_cas_protocol_runtime_check(store):
    from trajectory_ir.storage.cas import CAS

    assert isinstance(store, CAS)


def test_put_creates_sharded_dirs(store):
    h = store.put(os.urandom(32))
    assert (store.root / "cas" / h[:2]).is_dir()
