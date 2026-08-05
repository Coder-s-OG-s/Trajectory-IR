"""S3 compatible content addressed store (server-s3 profile).

Implements the same :class:`~trajectory_ir.storage.cas.CAS` protocol as
:class:`~trajectory_ir.storage.fs.FileSystemCAS`, using the sharded key layout
from README section 11.2.

Requires the optional ``boto3`` dependency::

    pip install boto3
    # or: pip install -e ".[s3]"

Credentials and endpoint come from the environment or an injected client.
Never hardcode secrets in repository code.
"""

from drivers.s3.cas import S3CAS, build_s3_client_from_env

__all__ = [
    "S3CAS",
    "build_s3_client_from_env",
]
