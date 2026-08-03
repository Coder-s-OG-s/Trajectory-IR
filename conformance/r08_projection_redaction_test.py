"""Conformance R08: redaction removes secrets from projected context."""

from __future__ import annotations

from trajectory_ir.runtime.redact import redact_projection_context


def test_r08_redacts_secret_shaped_values_in_items() -> None:
    context = {
        "items": [
            {
                "id": "n1",
                "kind": "TOOL_CALL",
                "payload": {
                    "tool": "note",
                    "args": {
                        "note": "found it: AKIAABCDEFGHIJKLMNOP",
                        "message": "benign",
                    },
                },
                "step_n": 1,
                "seq": 2,
            },
            {
                "id": "c1",
                "kind": "CONSTRAINT",
                "payload": {"rule": "always-include", "api_key": "should-scrub"},
                "step_n": 1,
                "seq": 0,
            },
            {
                "id": "th1",
                "kind": "THOUGHT",
                "payload": {"text": "private chain of thought"},
                "step_n": 1,
                "seq": 1,
            },
        ],
        "included_ids": ["c1", "th1", "n1"],
        "budget": 99999,
        "metric": "rfc8785_bytes",
    }
    out = redact_projection_context(context)
    assert out["redacted"] is True
    by_id = {item["id"]: item for item in out["items"]}
    # CONSTRAINT node remains present (R04) but secret field scrubbed.
    assert "c1" in by_id
    assert by_id["c1"]["kind"] == "CONSTRAINT"
    assert by_id["c1"]["payload"]["rule"] == "always-include"
    assert by_id["c1"]["payload"]["api_key"] == "[REDACTED]"
    # Secret-shaped free text scrubbed.
    assert by_id["n1"]["payload"]["args"]["note"] == "[REDACTED]"
    assert by_id["n1"]["payload"]["args"]["message"] == "benign"
    # THOUGHT body not projected as raw text.
    assert by_id["th1"]["payload"] == {"redacted": True}
    assert "private chain" not in str(out)


def test_r08_constraint_still_listed_after_redaction() -> None:
    context = {
        "items": [
            {
                "id": "c-only",
                "kind": "CONSTRAINT",
                "payload": {"rule": "keep-me"},
                "step_n": 0,
                "seq": 0,
            }
        ],
        "included_ids": ["c-only"],
    }
    out = redact_projection_context(context)
    assert len(out["items"]) == 1
    assert out["items"][0]["id"] == "c-only"
    assert out["items"][0]["payload"]["rule"] == "keep-me"
