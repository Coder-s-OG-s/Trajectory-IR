import hashlib
import time
from dataclasses import dataclass, field

try:
    import rfc8785
except ImportError:
    rfc8785 = None  # type: ignore[assignment]

NODE_KINDS = frozenset(
    {
        "INPUT",
        "CONSTRAINT",
        "STATE_SET",
        "PROJECT_CONTEXT",
        "THOUGHT",
        "DECISION",
        "TOOL_CALL",
        "TOOL_RESULT",
        "ARTIFACT_PUT",
        "ARTIFACT_REF",
        "LTM_QUERY",
        "LTM_HIT",
        "LTM_PROJECT",
        "COMMIT_STEP",
        "ABORT",
        "REDACTION",
    }
)

# Identity fields are joined with '|'. Forbid that delimiter and control chars
# so tenant_id / trajectory_id cannot alias across the preimage boundary.
_FORBIDDEN_ID_CHARS = frozenset({"|", "\n", "\r", "\x00"})


class NodeValidationError(ValueError):
    """Raised when node fields violate IR identity or payload rules."""


def validate_id_component(name: str, value: str) -> str:
    """Validate a tenant_id or trajectory_id for safe node_id composition."""
    if not isinstance(value, str) or not value:
        raise NodeValidationError(f"{name} must be a non-empty string")
    if any(ch in value for ch in _FORBIDDEN_ID_CHARS):
        raise NodeValidationError(
            f"{name} must not contain '|' or control characters (identity delimiter safety)"
        )
    if len(value) > 512:
        raise NodeValidationError(f"{name} exceeds maximum length 512")
    return value


def payload_hash(payload: dict) -> str:
    """RFC 8785 canonicalize, then SHA-256. `ts` must never be hashed (spec §6.3)."""
    if not isinstance(payload, dict):
        raise NodeValidationError("payload must be an object")
    if "ts" in payload:
        raise NodeValidationError("wall-clock ts must never be hashed (spec §6.3)")
    if rfc8785 is None:
        raise RuntimeError("rfc8785 is required for payload hashing but is not installed")
    canon = rfc8785.dumps(payload)
    return hashlib.sha256(canon).hexdigest()


def node_id(
    tenant_id: str,
    trajectory_id: str,
    step_n: int | None,
    seq: int,
    kind: str,
    phash: str,
) -> str:
    validate_id_component("tenant_id", tenant_id)
    validate_id_component("trajectory_id", trajectory_id)
    raw = f"{tenant_id}|{trajectory_id}|{step_n}|{seq}|{kind}|{phash}".encode()
    return hashlib.sha256(raw).hexdigest()


@dataclass
class Node:
    kind: str
    trajectory_id: str
    tenant_id: str
    step_n: int | None
    seq: int
    payload: dict
    ts: float = field(default_factory=time.time)

    def __post_init__(self):
        if self.kind not in NODE_KINDS:
            raise NodeValidationError(f"unknown node kind: {self.kind}")
        validate_id_component("tenant_id", self.tenant_id)
        validate_id_component("trajectory_id", self.trajectory_id)
        self.phash = payload_hash(self.payload)
        self.id = node_id(
            self.tenant_id,
            self.trajectory_id,
            self.step_n,
            self.seq,
            self.kind,
            self.phash,
        )
