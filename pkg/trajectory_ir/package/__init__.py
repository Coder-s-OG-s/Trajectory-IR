"""Portable `.tir` package export and import."""

from trajectory_ir.package.errors import TirError
from trajectory_ir.package.signature import (
    SignatureInfo,
    SignerMeta,
    TirSignatureError,
    sign_package,
    verify_package,
)
from trajectory_ir.package.tir import (
    COMPAT,
    PACKAGE_FORMAT_VERSION,
    RUNTIME_VERSION,
    ArtifactRef,
    TirFormatError,
    TirLimitError,
    TirPackage,
    TirRuntimeError,
    TirVerificationError,
    export_tir,
    import_tir,
    load_tir,
    load_tir_unverified,
)

__all__ = [
    "COMPAT",
    "PACKAGE_FORMAT_VERSION",
    "RUNTIME_VERSION",
    "ArtifactRef",
    "SignatureInfo",
    "SignerMeta",
    "TirError",
    "TirFormatError",
    "TirLimitError",
    "TirPackage",
    "TirRuntimeError",
    "TirSignatureError",
    "TirVerificationError",
    "export_tir",
    "import_tir",
    "load_tir",
    "load_tir_unverified",
    "sign_package",
    "verify_package",
]
