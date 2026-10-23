"""Start trajir-console on 127.0.0.1:8787 when that port is free.

Any other address is refused. A listener that is already up is left alone.
"""

from __future__ import annotations

import os
import socket
import subprocess
import time
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

CONSOLE_ADDR = "127.0.0.1:8787"


@dataclass(frozen=True)
class ConsoleLaunch:
    status: str
    process: subprocess.Popen[bytes] | None = None


def console_is_up(addr: str = CONSOLE_ADDR, timeout: float = 0.3) -> bool:
    """Return True when something accepts TCP on the console address."""
    if addr != CONSOLE_ADDR:
        raise ValueError("console address must be 127.0.0.1:8787")
    host, port_text = addr.rsplit(":", 1)
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.settimeout(timeout)
        return sock.connect_ex((host, int(port_text))) == 0


def console_binary(root: Path) -> Path:
    """Return go/trajir-console.exe, building it when the file is missing."""
    exe = Path(root) / "go" / "trajir-console.exe"
    if exe.is_file():
        return exe
    exe.parent.mkdir(parents=True, exist_ok=True)
    subprocess.check_call(
        ["go", "build", "-o", str(exe), "./cmd/trajir-console"],
        cwd=str(Path(root) / "go"),
    )
    return exe


def start_console_if_free(
    root: Path,
    *,
    data_dir: Path | None = None,
    detach: bool = True,
    probe: Callable[[], bool] | None = None,
    popen: Callable[..., subprocess.Popen[bytes]] | None = None,
    confirm: bool = True,
    wait_s: float = 5.0,
) -> ConsoleLaunch:
    """Start the console, or report that 127.0.0.1:8787 is already taken.

    ``status`` is ``already``, ``started``, or ``down``. ``down`` means the
    process exited or never accepted a connection. ``confirm=False`` returns
    after spawn and is for tests that do not open the port.
    """
    listening = console_is_up if probe is None else probe
    if listening():
        return ConsoleLaunch("already")
    binary = console_binary(root)
    if data_dir is not None:
        data = Path(data_dir)
    else:
        data = Path(root) / ".local-evidence" / "console-data"
    data.mkdir(parents=True, exist_ok=True)
    cmd = [str(binary), "-addr", CONSOLE_ADDR, "-data", str(data)]
    cwd = str(Path(root) / "go")
    stdout = subprocess.DEVNULL
    stderr = subprocess.DEVNULL
    creationflags = 0
    start_new_session = False
    if detach:
        if os.name == "nt":
            creationflags = getattr(subprocess, "DETACHED_PROCESS", 0)
            creationflags |= getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
        else:
            start_new_session = True
    kwargs: dict[str, object] = {"cwd": cwd, "stdout": stdout, "stderr": stderr}
    if creationflags:
        kwargs["creationflags"] = creationflags
    if start_new_session:
        kwargs["start_new_session"] = True
    if popen is not None:
        process = popen(cmd, **kwargs)
    elif creationflags:
        process = subprocess.Popen(
            cmd,
            cwd=cwd,
            stdout=stdout,
            stderr=stderr,
            creationflags=creationflags,
        )
    elif start_new_session:
        process = subprocess.Popen(
            cmd,
            cwd=cwd,
            stdout=stdout,
            stderr=stderr,
            start_new_session=True,
        )
    else:
        process = subprocess.Popen(cmd, cwd=cwd, stdout=stdout, stderr=stderr)
    if not confirm:
        return ConsoleLaunch("started", process)
    deadline = time.time() + wait_s
    while time.time() < deadline:
        if listening():
            return ConsoleLaunch("started", process)
        if process.poll() is not None:
            return ConsoleLaunch("down", process)
        time.sleep(0.05)
    return ConsoleLaunch("down", process)
