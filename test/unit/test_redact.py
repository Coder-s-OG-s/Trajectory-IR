"""Unit coverage for trajectory_ir.runtime.redact (R08)."""

from __future__ import annotations

from trajectory_ir.runtime.redact import (
    REDACTED,
    redact_payload,
    redact_projection_context,
    redact_value,
)


def test_redact_value_by_key_name():
    assert redact_value("api_key", "secret-value") == REDACTED
    assert redact_value("password", "x") == REDACTED
    assert redact_value("note", "hello") == "hello"


def test_redact_value_by_shape():
    assert redact_value("note", "AKIAABCDEFGHIJKLMNOP") == REDACTED
    assert redact_value("note", "sk-" + "x" * 20) == REDACTED
    assert redact_value("note", "plain text") == "plain text"


def test_redact_nested_dict_and_list():
    out = redact_value(
        "body",
        {"token": "abc", "items": [{"password": "p"}, "ok"]},
    )
    assert out["token"] == REDACTED
    assert out["items"][0]["password"] == REDACTED
    assert out["items"][1] == "ok"


def test_redact_payload_thought():
    assert redact_payload("THOUGHT", {"text": "private"}) == {"redacted": True}


def test_redact_payload_constraint_keeps_structure():
    out = redact_payload("CONSTRAINT", {"rule": "keep", "secret": "nope"})
    assert out["rule"] == "keep"
    assert out["secret"] == REDACTED


def test_redact_projection_context_items():
    ctx = {
        "items": [
            {"id": "1", "kind": "THOUGHT", "payload": {"text": "x"}},
            {"id": "2", "kind": "CONSTRAINT", "payload": {"api_key": "k", "rule": "r"}},
        ],
        "budget": 10,
    }
    out = redact_projection_context(ctx)
    assert out["redacted"] is True
    assert out["budget"] == 10
    by_id = {i["id"]: i for i in out["items"]}
    assert by_id["1"]["payload"] == {"redacted": True}
    assert by_id["2"]["payload"]["api_key"] == REDACTED
    assert by_id["2"]["payload"]["rule"] == "r"


def test_redact_projection_context_free_form():
    out = redact_projection_context({"password": "x", "goal": "ship"})
    assert out["password"] == REDACTED
    assert out["goal"] == "ship"
