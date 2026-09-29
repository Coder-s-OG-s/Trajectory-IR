"""Demo scripts only prepare fixture data and print the local URL."""

from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_console_demo_scripts_prepare_fixture():
    sh = (ROOT / "scripts" / "console_demo.sh").read_text(encoding="utf-8")
    ps = (ROOT / "scripts" / "console_demo.ps1").read_text(encoding="utf-8")
    fixture = "go/trajir/console/testdata/console_demo.ndjson"
    url = "http://127.0.0.1:8787/?id=console-demo"
    for text in (sh, ps):
        assert "console_demo.ndjson" in text
        assert fixture.replace("/", "\\") in text or fixture in text or "console_demo.ndjson" in text
        assert url in text
        assert "trajir-console" in text
    assert sh.startswith("#!/usr/bin/env bash")
    assert "set -euo pipefail" in sh
    assert "Copy-Item" in ps
