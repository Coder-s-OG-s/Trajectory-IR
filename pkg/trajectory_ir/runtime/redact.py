"""Shared redaction heuristics for package export and context projection.

This is a keyword/pattern heuristic, not a general secret scanner. See
SECURITY.md. Used by:
- ``package/tir.py`` redacted export
- projection redaction (R08)
"""

from __future__ import annotations

import re
from typing import Any

# Keys whose values are redacted (case-insensitive substring match).
SECRET_KEY_RE = re.compile(
    r"(password|passwd|pass[_-]?phrase|secret|token|api[_-]?key|access[_-]?key"
    r"|authorization|auth|credential|private[_-]?key"
    r"|database[_-]?url|connection[_-]?string|db[_-]?url|dsn)",
    re.IGNORECASE,
)

# Value shapes that look like a secret regardless of field name.
SECRET_VALUE_RE = re.compile(
    r"(AKIA[0-9A-Z]{16}"  # AWS access key id
    r"|[Bb]earer\s+[A-Za-z0-9\-_.=]{10,}"  # bearer token
    r"|gh[pousr]_[A-Za-z0-9]{20,}"  # GitHub token
    r"|sk-[A-Za-z0-9]{20,}"  # OpenAI-style secret key
    r"|[sr]k_live_[A-Za-z0-9]{8,}"  # Stripe live secret/restricted key
    r"|[sr]k_test_[A-Za-z0-9]{8,}"  # Stripe test secret/restricted key
    r"|xox[baprs]-[A-Za-z0-9-]{10,}"  # Slack token
    r"|AIza[0-9A-Za-z\-_]{35}"  # GCP API key
    r"|-----BEGIN[ A-Z]*PRIVATE KEY-----"  # PEM private key block
    r"|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}"  # JWT
    r"|[a-zA-Z][a-zA-Z0-9+.-]*://[^:@\s]+:[^@\s]+@)"  # credential URI (scheme://user:pass@)
)

REDACTED = "[REDACTED]"


def redact_value(key: str, value: Any) -> Any:
    """Redact a single value by key name or secret-shaped string content."""
    if SECRET_KEY_RE.search(key):
        return REDACTED
    if isinstance(value, dict):
        return {k: redact_value(str(k), v) for k, v in value.items()}
    if isinstance(value, list):
        return [redact_value(key, item) for item in value]
    if isinstance(value, str) and SECRET_VALUE_RE.search(value):
        return REDACTED
    return value


def redact_payload(kind: str, payload: Any) -> dict[str, Any]:
    """Redact a node payload for projection or export.

    THOUGHT bodies are replaced with ``{\"redacted\": true}`` so private
    reasoning never enters projected context (R08) or shared packages.
    CONSTRAINT payloads are still redacted for secrets but keep structure
    (R04: never drop the node; only scrub fields).
    """
    if kind == "THOUGHT":
        return {"redacted": True}
    if isinstance(payload, dict):
        return {k: redact_value(str(k), v) for k, v in payload.items()}
    return {"redacted": True}


def redact_projection_context(context: dict[str, Any]) -> dict[str, Any]:
    """Return a copy of a projector-style context with secrets/thoughts scrubbed.

    Expected shape (from R04 ``ProjectResult.context`` or equivalent)::

        {\"items\": [{\"id\", \"kind\", \"payload\", ...}, ...], ...}

    CONSTRAINT items stay present; only payload fields are redacted in place.
    """
    items_in = context.get("items")
    if not isinstance(items_in, list):
        # Treat free-form context dict as a single redaction pass on top-level keys.
        return {k: redact_value(str(k), v) for k, v in context.items()}

    new_items: list[dict[str, Any]] = []
    for item in items_in:
        if not isinstance(item, dict):
            continue
        kind = str(item.get("kind") or "")
        payload = item.get("payload")
        new_item = dict(item)
        new_item["payload"] = redact_payload(kind, payload)
        new_items.append(new_item)

    out = dict(context)
    out["items"] = new_items
    out["redacted"] = True
    return out
