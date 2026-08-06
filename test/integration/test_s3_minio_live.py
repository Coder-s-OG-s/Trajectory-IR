"""Live S3 CAS tests against MinIO or any S3 compatible endpoint.

Requires:

* ``TRAJIR_S3_ENDPOINT_URL`` (e.g. http://127.0.0.1:9000)
* ``TRAJIR_S3_BUCKET`` (bucket must exist or be creatable)
* ``AWS_ACCESS_KEY_ID`` / ``AWS_SECRET_ACCESS_KEY``
* optional ``AWS_REGION`` (default us-east-1)
"""

from __future__ import annotations

import os
import uuid

import pytest

pytest.importorskip("boto3")

from drivers.s3.cas import S3CAS, build_s3_client_from_env
from trajectory_ir.storage.cas import CASIntegrityError, CASNotFoundError, content_hash
from trajectory_ir.storage.rehydrate import rehydrate_artifacts

pytestmark = pytest.mark.skipif(
    not os.environ.get("TRAJIR_S3_ENDPOINT_URL") or not os.environ.get("TRAJIR_S3_BUCKET"),
    reason="Set TRAJIR_S3_ENDPOINT_URL and TRAJIR_S3_BUCKET for MinIO/S3 integration",
)


@pytest.fixture(scope="module")
def bucket() -> str:
    return os.environ["TRAJIR_S3_BUCKET"]


@pytest.fixture(scope="module")
def client(bucket: str):
    client = build_s3_client_from_env()
    # Ensure bucket exists (idempotent for MinIO).
    try:
        client.head_bucket(Bucket=bucket)
    except Exception:
        client.create_bucket(Bucket=bucket)
    return client


@pytest.fixture
def store(client, bucket: str) -> S3CAS:
    return S3CAS(client, bucket, key_prefix=f"ci/{uuid.uuid4().hex[:8]}")


def test_live_put_get_has_roundtrip(store: S3CAS):
    data = b"minio live artifact " + uuid.uuid4().bytes
    h = store.put(data)
    assert h == content_hash(data)
    assert store.has(h)
    assert store.get(h) == data
    assert store.uri_for(h).startswith(f"s3://{store.bucket}/")


def test_live_put_idempotent(store: S3CAS):
    data = b"same-bytes"
    h1 = store.put(data)
    h2 = store.put(data)
    assert h1 == h2


def test_live_get_missing(store: S3CAS):
    h = content_hash(b"not-uploaded-" + uuid.uuid4().bytes)
    with pytest.raises(CASNotFoundError):
        store.get(h)
    assert store.has(h) is False


def test_live_rehydrate(store: S3CAS):
    h = store.put(b"rehydrate-me")
    got = rehydrate_artifacts(
        store,
        [{"content_hash": h, "logical_path": "out.bin"}],
    )
    assert got[h] == b"rehydrate-me"


def test_live_get_detects_tamper(store: S3CAS, client, bucket: str):
    h = store.put(b"honest")
    key = store.object_key(h)
    client.put_object(Bucket=bucket, Key=key, Body=b"tampered")
    with pytest.raises(CASIntegrityError):
        store.get(h)
