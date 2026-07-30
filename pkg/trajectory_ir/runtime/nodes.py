import hashlib
import time
from dataclasses import dataclass, field

import rfc8785

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


def payload_hash(payload: dict) -> str:
    """RFC 8785 canonicalize, then SHA-256. `ts` must never be hashed (spec §6.3)."""
    assert "ts" not in payload, "wall-clock ts must never be hashed (spec §6.3)"
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
        assert self.kind in NODE_KINDS, f"unknown node kind: {self.kind}"
        self.phash = payload_hash(self.payload)
        self.id = node_id(
            self.tenant_id,
            self.trajectory_id,
            self.step_n,
            self.seq,
            self.kind,
            self.phash,
        )
