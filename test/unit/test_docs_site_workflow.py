"""The website copy of the docs workflow stays the same file as Actions runs."""

from __future__ import annotations

from pathlib import Path


def test_docs_site_workflow_copy_matches() -> None:
    root = Path(__file__).resolve().parents[2]
    live = (root / ".github" / "workflows" / "docs-site.yml").read_bytes()
    copy = (root / "website" / "docs-site.workflow.yml").read_bytes()
    assert live == copy
    text = live.decode("utf-8").replace("\r\n", "\n")
    assert "github.ref != 'refs/heads/main'" in text
    pages_if = (
        "github.ref == 'refs/heads/main' && github.event_name != 'pull_request' "
        "&& vars.ENABLE_PAGES == 'true'"
    )
    assert (f"      - name: Upload Pages artifact\n        if: {pages_if}\n") in text
    assert (f"  deploy:\n    if: {pages_if}\n") in text
