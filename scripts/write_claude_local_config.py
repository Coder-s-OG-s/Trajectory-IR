"""Write the project-local Claude hook and MCP config. Does not touch the user profile.

Existing JSON object files are merged. A file that is not a JSON object is
left unchanged and the command exits 1. Pass --target to write into another
project. Pass --skip-console to leave 127.0.0.1:8787 alone.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[1]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))

from integrations.attach.console_launch import start_console_if_free  # noqa: E402
from integrations.attach.project_config import write_local_config  # noqa: E402


def _mcp_binary(root: Path) -> Path:
    exe = root / "go" / "trajir-mcp.exe"
    if exe.is_file():
        return exe
    plain = root / "go" / "trajir-mcp"
    return plain if plain.is_file() else exe


def _parse(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Write project-local Trajectory IR config")
    parser.add_argument("--target", default="", help="project directory to configure")
    parser.add_argument(
        "--skip-console",
        action="store_true",
        help="do not start trajir-console on 127.0.0.1:8787",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse(argv)
    target = Path(args.target) if args.target else None
    code = write_local_config(_ROOT, sys.executable, _mcp_binary(_ROOT), target=target)
    if code != 0:
        return code
    if args.skip_console:
        print("console start skipped")
        return 0
    dest = Path(args.target).resolve() if args.target else _ROOT
    data = dest / ".local-evidence" / "console-data"
    launch = start_console_if_free(_ROOT, data_dir=data)
    if launch.status == "already":
        print("console already on 127.0.0.1:8787; left it alone")
        return 0
    if launch.status == "started":
        print(f"console started on 127.0.0.1:8787 data={data}")
        return 0
    print("console did not stay up on 127.0.0.1:8787", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
