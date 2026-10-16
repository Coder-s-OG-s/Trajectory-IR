from __future__ import annotations

import subprocess
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_wheel_ships_client_and_drivers_under_trajectory_ir(tmp_path: Path) -> None:
    subprocess.run(
        [sys.executable, "-m", "pip", "wheel", str(ROOT), "--no-deps", "-w", str(tmp_path)],
        check=True,
    )
    wheels = list(tmp_path.glob("trajectory_ir-*.whl"))
    assert len(wheels) == 1
    names = zipfile.ZipFile(wheels[0]).namelist()
    tops = {name.split("/", 1)[0] for name in names}
    assert "client" not in tops
    assert "drivers" not in tops
    assert any(name.startswith("trajectory_ir/client/") for name in names)
    assert any(name.startswith("trajectory_ir/drivers/") for name in names)
