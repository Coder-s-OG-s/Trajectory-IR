import subprocess
import sys


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
