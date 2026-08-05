"""S3 compatible CAS backend.

Key layout (identical to FileSystemCAS)::

    cas/<first-2-hex>/<full-sha256-hex>

The client is injected so unit tests can use a fake without AWS or MinIO.
Production code typically calls :func:`build_s3_client_from_env`.
"""

from __future__ import annotations

import os
from typing import Any, Protocol

from trajectory_ir.storage.cas import (
    CASIntegrityError,
    CASNotFoundError,
    content_hash,
    normalize_content_hash,
    shard_key,
)


class S3ClientProtocol(Protocol):
    """Minimal subset of boto3 S3 client methods used by :class:`S3CAS`."""

    def put_object(self, *, Bucket: str, Key: str, Body: bytes, **kwargs: Any) -> Any: ...

    def get_object(self, *, Bucket: str, Key: str, **kwargs: Any) -> Any: ...

    def head_object(self, *, Bucket: str, Key: str, **kwargs: Any) -> Any: ...


class S3CAS:
    """Content addressed store backed by an S3 compatible API.

    Args:
        client: boto3 style S3 client (or test double).
        bucket: Bucket name (must already exist).
        key_prefix: Optional prefix before ``cas/...`` (no leading/trailing
            slash required; empty string is fine).
    """

    def __init__(
        self,
        client: S3ClientProtocol,
        bucket: str,
        *,
        key_prefix: str = "",
    ) -> None:
        if not bucket:
            raise ValueError("bucket is required")
        self._client = client
        self._bucket = bucket
        self._prefix = key_prefix.strip("/")

    @property
    def bucket(self) -> str:
        return self._bucket

    def object_key(self, content_hash_hex: str) -> str:
        """Full object key including optional prefix."""
        rel = shard_key(content_hash_hex)
        if self._prefix:
            return f"{self._prefix}/{rel}"
        return rel

    def uri_for(self, content_hash_hex: str) -> str:
        """``s3://bucket/key`` URI for thin package artifact refs."""
        h = normalize_content_hash(content_hash_hex)
        return f"s3://{self._bucket}/{self.object_key(h)}"

    def put(self, data: bytes) -> str:
        if not isinstance(data, (bytes, bytearray)):
            raise TypeError("put expects bytes")
        data = bytes(data)
        h = content_hash(data)
        key = self.object_key(h)
        if self.has(h):
            # Idempotent: existing object must match hash on next get.
            return h
        self._client.put_object(Bucket=self._bucket, Key=key, Body=data)
        return h

    def get(self, content_hash_hex: str) -> bytes:
        h = normalize_content_hash(content_hash_hex)
        key = self.object_key(h)
        try:
            resp = self._client.get_object(Bucket=self._bucket, Key=key)
        except Exception as exc:
            # boto3 raises ClientError with Code NoSuchKey; fakes may raise KeyError.
            name = type(exc).__name__
            code = ""
            if hasattr(exc, "response"):
                code = str(exc.response.get("Error", {}).get("Code", ""))  # type: ignore[union-attr]
            if (
                name in {"NoSuchKey", "ClientError", "KeyError"}
                or code in {"NoSuchKey", "404", "NotFound"}
                or "NoSuchKey" in str(exc)
                or "404" in str(exc)
            ):
                raise CASNotFoundError(f"no object for content_hash={h}") from exc
            raise
        body = resp["Body"]
        data = body.read() if hasattr(body, "read") else bytes(body)
        actual = content_hash(data)
        if actual != h:
            raise CASIntegrityError(
                f"s3://{self._bucket}/{key} failed hash verify: expected {h}, got {actual}"
            )
        return data

    def has(self, content_hash_hex: str) -> bool:
        h = normalize_content_hash(content_hash_hex)
        key = self.object_key(h)
        try:
            self._client.head_object(Bucket=self._bucket, Key=key)
            return True
        except Exception:
            return False


def build_s3_client_from_env() -> Any:
    """Build a boto3 S3 client from environment variables.

    Recognized variables:

    * ``AWS_ACCESS_KEY_ID`` / ``AWS_SECRET_ACCESS_KEY`` (standard)
    * ``AWS_REGION`` or ``AWS_DEFAULT_REGION`` (default ``us-east-1``)
    * ``TRAJIR_S3_ENDPOINT_URL`` optional custom endpoint (MinIO, LocalStack)

    Raises:
        ImportError: if boto3 is not installed
        ValueError: if required configuration is missing in a strict way
    """
    try:
        import boto3
    except ImportError as exc:
        raise ImportError(
            "boto3 is required for S3CAS production use; pip install boto3 "
            'or pip install -e ".[s3]"'
        ) from exc

    region = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
    endpoint = os.environ.get("TRAJIR_S3_ENDPOINT_URL") or None
    return boto3.client("s3", region_name=region, endpoint_url=endpoint)
