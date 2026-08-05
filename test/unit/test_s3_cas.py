"""Unit tests for S3 CAS using an in memory fake client (no AWS)."""

from __future__ import annotations

from drivers.s3.cas import S3CAS
from trajectory_ir.storage.cas import CAS, CASIntegrityError, CASNotFoundError, content_hash
from trajectory_ir.storage.rehydrate import rehydrate_artifacts


class _FakeBody:
    def __init__(self, data: bytes) -> None:
        self._data = data

    def read(self) -> bytes:
        return self._data


class FakeS3Client:
    """Minimal S3 client double for unit tests."""

    def __init__(self) -> None:
        self.objects: dict[tuple[str, str], bytes] = {}

    def put_object(self, *, Bucket: str, Key: str, Body: bytes, **kwargs):
        self.objects[(Bucket, Key)] = bytes(Body)
        return {}

    def get_object(self, *, Bucket: str, Key: str, **kwargs):
        try:
            data = self.objects[(Bucket, Key)]
        except KeyError as exc:
            err = KeyError("NoSuchKey")
            raise err from exc
        return {"Body": _FakeBody(data)}

    def head_object(self, *, Bucket: str, Key: str, **kwargs):
        if (Bucket, Key) not in self.objects:
            raise KeyError("NoSuchKey")
        return {}


def test_put_get_roundtrip():
    client = FakeS3Client()
    store = S3CAS(client, "trajir-test")
    data = b"s3 artifact"
    h = store.put(data)
    assert h == content_hash(data)
    assert store.has(h)
    assert store.get(h) == data
    assert store.object_key(h) == f"cas/{h[:2]}/{h}"
    assert store.uri_for(h) == f"s3://trajir-test/cas/{h[:2]}/{h}"


def test_key_prefix():
    client = FakeS3Client()
    store = S3CAS(client, "b", key_prefix="tenant-a")
    h = store.put(b"x")
    assert store.object_key(h).startswith("tenant-a/cas/")
    assert (store.bucket, store.object_key(h)) in client.objects


def test_put_idempotent():
    client = FakeS3Client()
    store = S3CAS(client, "b")
    h1 = store.put(b"same")
    h2 = store.put(b"same")
    assert h1 == h2
    assert len(client.objects) == 1


def test_get_missing():
    store = S3CAS(FakeS3Client(), "b")
    h = content_hash(b"gone")
    try:
        store.get(h)
        raise AssertionError("expected CASNotFoundError")
    except CASNotFoundError:
        pass
    assert not store.has(h)


def test_get_detects_tamper():
    client = FakeS3Client()
    store = S3CAS(client, "b")
    h = store.put(b"good")
    key = store.object_key(h)
    client.objects[("b", key)] = b"bad"
    try:
        store.get(h)
        raise AssertionError("expected CASIntegrityError")
    except CASIntegrityError:
        pass


def test_rehydrate_via_s3_cas():
    store = S3CAS(FakeS3Client(), "b")
    h = store.put(b"payload")
    got = rehydrate_artifacts(store, [{"content_hash": h, "logical_path": "f.bin"}])
    assert got[h] == b"payload"


def test_implements_cas_protocol():
    store = S3CAS(FakeS3Client(), "b")
    assert isinstance(store, CAS)


def test_bucket_required():
    try:
        S3CAS(FakeS3Client(), "")
        raise AssertionError("expected ValueError")
    except ValueError:
        pass
