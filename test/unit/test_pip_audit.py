"""CI dependency vulnerability gate (Python parity with Go govulncheck).

Runs under GitHub Actions (CI=true). Locally: ``RUN_PIP_AUDIT=1 pytest test/unit/test_pip_audit.py``.
Skip with ``SKIP_PIP_AUDIT=1`` when needed.
"""

from __future__ import annotations

import os
import subprocess
import sys

import pytest


def test_installed_dependency_tree_has_no_known_vulns() -> None:
    """Fail when pip-audit reports known vulnerabilities in the installed tree."""
    if os.environ.get("SKIP_PIP_AUDIT") == "1":
        pytest.skip("SKIP_PIP_AUDIT=1")
    if os.environ.get("CI") != "true" and os.environ.get("RUN_PIP_AUDIT") != "1":
        pytest.skip("set RUN_PIP_AUDIT=1 to run pip-audit outside CI")

    # Ensure audit tooling and known-vulnerable build backends are current.
    # Hosted Python 3.11 images can ship an older setuptools that pip-audit flags.
    install = subprocess.run(
        [
            sys.executable,
            "-m",
            "pip",
            "install",
            "-q",
            "-U",
            "pip",
            "setuptools>=83",
            "pip-audit",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    if install.returncode != 0:
        pytest.fail(f"could not install pip-audit tooling:\n{install.stdout}\n{install.stderr}")

    # --skip-editable: ignore the local trajectory-ir package (not on PyPI).
    audit = subprocess.run(
        [sys.executable, "-m", "pip_audit", "--skip-editable"],
        capture_output=True,
        text=True,
        check=False,
    )
    if audit.returncode != 0:
        pytest.fail(
            "pip-audit found known vulnerabilities (or failed).\n"
            f"stdout:\n{audit.stdout}\nstderr:\n{audit.stderr}"
        )
