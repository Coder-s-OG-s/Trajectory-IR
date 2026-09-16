import subprocess
import sys
import tomllib
from pathlib import Path


def test_rfc8785_declared_in_core_dependencies():
    """Ensure rfc8785 is in project.dependencies so standard wheel installs include it."""
    pyproject_path = Path(__file__).resolve().parents[2] / "pyproject.toml"
    with open(pyproject_path, "rb") as f:
        data = tomllib.load(f)
    deps = data.get("project", {}).get("dependencies", [])
    assert any("rfc8785" in dep for dep in deps), (
        "rfc8785 must be declared in core project.dependencies"
    )


def test_payload_hash_and_size_units_with_rfc8785():
    """Verify standard payload hashing and size units operate normally when installed."""
    import rfc8785  # noqa: F401

    from trajectory_ir.runtime.nodes import payload_hash
    from trajectory_ir.runtime.projector import node_size_units

    digest = payload_hash({"type": "test", "active": True})
    assert isinstance(digest, str) and len(digest) == 64

    units = node_size_units({"kind": "INPUT", "payload": {"type": "test", "active": True}})
    assert isinstance(units, int) and units > 0


def test_nodes_import_succeeds_without_rfc8785():
    """Verify that importing nodes.py does not crash if rfc8785 is absent."""
    code = (
        "import sys\n"
        "sys.modules['rfc8785'] = None\n"
        "from trajectory_ir.runtime.nodes import payload_hash\n"
        "print('import success')\n"
    )
    result = subprocess.run([sys.executable, "-c", code], capture_output=True, text=True)
    assert result.returncode == 0, f"Import failed: {result.stderr}"
    assert "import success" in result.stdout

    # Now verify payload_hash actually raises RuntimeError when used
    code_raise = (
        "import sys\n"
        "sys.modules['rfc8785'] = None\n"
        "from trajectory_ir.runtime.nodes import payload_hash\n"
        "payload_hash({'kind': 'INPUT', 'payload': {}})\n"
    )
    result_raise = subprocess.run(
        [sys.executable, "-c", code_raise], capture_output=True, text=True
    )
    assert result_raise.returncode != 0
    assert "rfc8785 is required for payload hashing but is not installed" in result_raise.stderr


def test_projector_import_succeeds_without_rfc8785():
    """Verify that importing projector.py does not crash if rfc8785 is absent."""
    code = (
        "import sys\n"
        "sys.modules['rfc8785'] = None\n"
        "from trajectory_ir.runtime.projector import node_size_units\n"
        "print('import success')\n"
    )
    result = subprocess.run([sys.executable, "-c", code], capture_output=True, text=True)
    assert result.returncode == 0, f"Import failed: {result.stderr}"
    assert "import success" in result.stdout

    # Now verify node_size_units actually raises RuntimeError when used
    code_raise = (
        "import sys\n"
        "sys.modules['rfc8785'] = None\n"
        "from trajectory_ir.runtime.projector import node_size_units\n"
        "node_size_units({'kind': 'INPUT', 'payload': {}})\n"
    )
    result_raise = subprocess.run(
        [sys.executable, "-c", code_raise], capture_output=True, text=True
    )
    assert result_raise.returncode != 0
    assert "rfc8785 is required for size measurement but is not installed" in result_raise.stderr
