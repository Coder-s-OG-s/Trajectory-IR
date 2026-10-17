"""Write the local projection experiment. Does not call a model provider."""

from __future__ import annotations

import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parents[1]
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
if str(_ROOT / "pkg") not in sys.path:
    sys.path.insert(0, str(_ROOT / "pkg"))

from integrations.projection_eval.experiment import render_markdown, run_experiment  # noqa: E402


def main() -> int:
    dest = _ROOT / "docs" / "LOCAL_PROJECTION_EXPERIMENT.md"
    dest.parent.mkdir(parents=True, exist_ok=True)
    dest.write_text(render_markdown(run_experiment()), encoding="utf-8")
    print(dest)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
