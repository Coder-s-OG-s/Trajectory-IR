"""One fixed diary, two projection arms, three honest columns.

Task score is whether the sealed constraint text is still present.
Provider token counts are not requested from a model, so they stay none.
The console figure is ceil(chars/4) on the projected context JSON.
"""

from __future__ import annotations

import json
from typing import Any

from trajectory_ir.runtime.projector import project_context


ANSWER = "NORTH"


def estimated_tokens(char_len: int) -> int | None:
    if char_len < 0:
        return None
    if char_len == 0:
        return 0
    return (char_len + 3) // 4


def _nodes() -> list[dict[str, Any]]:
    filler = "note " * 400
    nodes: list[dict[str, Any]] = [
        {
            "id": "constraint-1",
            "kind": "CONSTRAINT",
            "step_n": 1,
            "seq": 0,
            "payload": {"text": f"The recovery code is {ANSWER}"},
        }
    ]
    for i in range(6):
        nodes.append(
            {
                "id": f"state-{i}",
                "kind": "STATE_SET",
                "step_n": 1,
                "seq": i + 1,
                "payload": {"note": f"{i} {filler}"},
            }
        )
    return nodes


def _arm(name: str, nodes: list[dict[str, Any]], budget: int) -> dict[str, Any]:
    projected = project_context(nodes, budget=budget)
    raw = json.dumps(projected.context, sort_keys=True, separators=(",", ":"))
    score = 1 if ANSWER in raw else 0
    return {
        "arm": name,
        "budget": budget,
        "task_score": score,
        "provider_prompt_tokens": None,
        "provider_completion_tokens": None,
        "console_estimate": estimated_tokens(len(raw)),
        "char_len": len(raw),
        "included": len(projected.included_ids),
        "dropped": len(projected.dropped_ids),
    }


def run_experiment() -> dict[str, Any]:
    nodes = _nodes()
    full = _arm("whole diary", nodes, budget=10_000_000)
    # Keep the constraint. Drop the long notes when they do not fit.
    small_budget = 180
    small = _arm("projection budget", nodes, budget=small_budget)
    winner = None
    if (
        small["provider_prompt_tokens"] is not None
        and full["provider_prompt_tokens"] is not None
        and small["task_score"] >= full["task_score"]
        and small["provider_prompt_tokens"] < full["provider_prompt_tokens"]
    ):
        winner = "projection budget"
    return {
        "answer": ANSWER,
        "arms": [full, small],
        "winner_on_provider_tokens": winner,
        "note": (
            "Provider prompt_tokens and completion_tokens were not sent, so they stay none. "
            "The console estimate is ceil of the projected context's character length divided by 4. "
            "A lower estimate is not a savings claim."
        ),
    }


def render_markdown(result: dict[str, Any]) -> str:
    lines = [
        "# Local projection experiment",
        "",
        "One fixed diary. Two arms. No model call.",
        "",
        "| Arm | Task score | Provider prompt_tokens | Provider completion_tokens | Console estimate |",
        "| --- | --- | --- | --- | --- |",
    ]
    for arm in result["arms"]:
        lines.append(
            "| {arm} | {task_score} | none | none | {console_estimate} |".format(
                arm=arm["arm"],
                task_score=arm["task_score"],
                console_estimate=arm["console_estimate"],
            )
        )
    lines.extend(
        [
            "",
            result["note"],
            "",
            "Task score is 1 when the constraint text NORTH is still in the projected context.",
            "CONSTRAINT is never dropped to fit a budget. If it cannot fit, projection raises BUDGET_IMPOSSIBLE.",
            "",
            "Winner on provider tokens: none. The provider columns were not measured.",
            "",
        ]
    )
    return "\n".join(lines)
