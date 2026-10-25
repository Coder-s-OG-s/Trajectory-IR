"""Launch a Claude hook with a project identity file.

argv: identity_path script_path kind [owner]
kind is "pre" or "post". The script must be one of the three Claude hook
files in this install. A launcher error on pre exits 2. On post it exits 0
so a broken launcher does not ask the host to run the tool again. Omitting
the owner is the same as an empty owner.
"""

from __future__ import annotations

import os
import runpy
import sys
from pathlib import Path

# The launcher only runs these three files from this install. A path from
# argv is not a hook, and it must not put some other directory on sys.path.
_ALLOWED_HOOKS = (
    Path("integrations") / "claude" / "pre_tool_use.py",
    Path("integrations") / "claude" / "post_tool_use.py",
    Path("integrations") / "claude" / "post_tool_use_failure.py",
)


def _fail_code(kind: str) -> int:
    return 0 if kind == "post" else 2


def _exit_code(exc: SystemExit, kind: str) -> int:
    code = exc.code
    if code is None or code == 0:
        return 0
    if isinstance(code, int):
        return code
    return _fail_code(kind)


def _repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def _allowed_script(script: str) -> Path:
    root = _repo_root()
    script_path = Path(script).resolve()
    allowed = {(root / relative).resolve() for relative in _ALLOWED_HOOKS}
    if script_path not in allowed or not script_path.is_file():
        raise ValueError(
            "hook script must be pre_tool_use.py, post_tool_use.py, "
            "or post_tool_use_failure.py in this install"
        )
    return script_path


def _prepare(identity: str, script: str) -> Path:
    script_path = _allowed_script(script)
    repo = _repo_root()
    os.environ["TRAJIR_IDENTITY_FILE"] = str(Path(identity).resolve())
    for entry in (str(repo), str(repo / "pkg")):
        if entry not in sys.path:
            sys.path.insert(0, entry)
    return script_path


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    kind = args[2] if len(args) >= 3 else "pre"
    if len(args) not in (3, 4):
        print("trajir hook launcher: need identity, script, and kind", file=sys.stderr)
        return _fail_code(kind)
    identity, script, kind = args[0], args[1], args[2]
    owner = args[3] if len(args) == 4 else ""
    previous = os.environ.get("TRAJIR_IDENTITY_FILE")
    previous_owner = os.environ.get("TRAJIR_HOOK_OWNER")
    try:
        script_path = _prepare(identity, script)
        if owner:
            os.environ["TRAJIR_HOOK_OWNER"] = owner
        else:
            os.environ.pop("TRAJIR_HOOK_OWNER", None)
        try:
            runpy.run_path(str(script_path), run_name="__main__")
        except SystemExit as exc:
            return _exit_code(exc, kind)
    except Exception as exc:
        print(f"trajir hook launcher: {exc}", file=sys.stderr)
        return _fail_code(kind)
    finally:
        if previous is None:
            os.environ.pop("TRAJIR_IDENTITY_FILE", None)
        else:
            os.environ["TRAJIR_IDENTITY_FILE"] = previous
        if previous_owner is None:
            os.environ.pop("TRAJIR_HOOK_OWNER", None)
        else:
            os.environ["TRAJIR_HOOK_OWNER"] = previous_owner
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
