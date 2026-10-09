from collections.abc import Callable, Mapping
from typing import Any

from trajectory_ir.effects import assert_open_world_effect, requires_block_and_gate
from trajectory_ir.resume.gate import make_gated_tool_call, make_plain_tool_call
from trajectory_ir.resume.world import check_world, decision_payload, sealed_world_snapshot
from trajectory_ir.runtime.sandbox import RunMode, assert_tool_allowed_in_mode, normalize_run_mode


def _resolve_durable_hooks(
    durable_infer_fn: Callable[[Callable[..., Any]], Callable[..., Any]] | None,
    durable_tool_fn: Callable[[Callable[..., Any]], Callable[..., Any]] | None,
    durable_workflow_fn: Callable[[Callable[..., Any]], Callable[..., Any]] | None,
) -> tuple[
    Callable[[Callable[..., Any]], Callable[..., Any]],
    Callable[[Callable[..., Any]], Callable[..., Any]],
    Callable[[Callable[..., Any]], Callable[..., Any]],
]:
    """Resolve durable wrappers; refuse silent DBOS/Restate (or other) mixes.

    Rules:
    1. All three None → default DBOS adapter hooks.
    2. All three set → use the injected set as a single backend.
    3. Any other combination → ValueError (fail loud).
    """
    provided = (
        durable_infer_fn is not None,
        durable_tool_fn is not None,
        durable_workflow_fn is not None,
    )
    if any(provided) and not all(provided):
        missing = [
            name
            for name, set_ in (
                ("durable_infer_fn", durable_infer_fn is not None),
                ("durable_tool_fn", durable_tool_fn is not None),
                ("durable_workflow_fn", durable_workflow_fn is not None),
            )
            if not set_
        ]
        raise ValueError(
            "make_run_step durable hooks must be injected together "
            f"(all three or none); missing: {', '.join(missing)}. "
            "Partial injection would silently mix durable backends."
        )
    if all(provided):
        assert durable_infer_fn is not None
        assert durable_tool_fn is not None
        assert durable_workflow_fn is not None
        return durable_infer_fn, durable_tool_fn, durable_workflow_fn

    from drivers.durable_backend.dbos.adapter import (
        durable_infer as dbos_infer,
    )
    from drivers.durable_backend.dbos.adapter import (
        durable_tool as dbos_tool,
    )
    from drivers.durable_backend.dbos.adapter import (
        durable_workflow as dbos_workflow,
    )

    return dbos_infer, dbos_tool, dbos_workflow


def _decision_world_snapshot(
    node_log,
    trajectory_id: str,
    tenant_id: str,
    step_n: int,
    world_snapshot: Mapping[str, str] | Callable[[int], Mapping[str, str]] | None,
) -> Mapping[str, str] | None:
    """Snapshot to seal. An existing DECISION wins over the host's current value."""
    sealed = sealed_world_snapshot(node_log, trajectory_id, tenant_id, step_n)
    if sealed is not None:
        return sealed or None
    if callable(world_snapshot):
        return world_snapshot(step_n)
    return world_snapshot


def _require_plan_object(plan: Any) -> dict:
    if not isinstance(plan, dict):
        raise ValueError(f"resume: plan: want object, got {type(plan).__name__}")
    return plan


def _tool_calls_from_plan(plan: dict) -> list[tuple[str, dict]]:
    if "tool_calls" not in plan:
        raise ValueError("resume: plan missing tool_calls")
    raw = plan["tool_calls"]
    if not isinstance(raw, list):
        raise ValueError("resume: tool_calls must be a list")
    calls: list[tuple[str, dict]] = []
    for i, item in enumerate(raw):
        if not isinstance(item, dict):
            got = type(item).__name__
            raise ValueError(f"resume: tool_calls[{i}]: want object, got {got}")
        name = item.get("name")
        if not isinstance(name, str) or name == "":
            raise ValueError(f"resume: tool_calls[{i}] missing name")
        if "args" in item and item["args"] is not None:
            raw_args = item["args"]
            if not isinstance(raw_args, dict):
                got = type(raw_args).__name__
                raise ValueError(f"resume: tool_calls[{i}].args: want object, got {got}")
            args = raw_args
        else:
            args = {}
        calls.append((name, args))
    return calls


def make_run_step(
    node_log,
    tenant_id,
    trajectory_id,
    tool_registry,
    on_decision_sealed=None,
    *,
    mode: RunMode | str = RunMode.LIVE,
    world_snapshot: Mapping[str, str] | Callable[[int], Mapping[str, str]] | None = None,
    observe_world: Callable[[], Mapping[str, str]] | None = None,
    durable_infer_fn: Callable[[Callable[..., Any]], Callable[..., Any]] | None = None,
    durable_tool_fn: Callable[[Callable[..., Any]], Callable[..., Any]] | None = None,
    durable_workflow_fn: Callable[[Callable[..., Any]], Callable[..., Any]] | None = None,
):
    """Build a durable run_step workflow for one agent step.

    Resume matrix (README §8, R02 / R03):
    - ``NON_IDEMPOTENT_WRITE``: block-and-gate via ``make_gated_tool_call`` (R02).
    - ``PURE`` and other non-gated classes: no gate; body may re-run on resume
      when durable memo is absent (R03 for PURE). Prior TOOL_CALL rows alone
      must not raise ``BlockedNeedsGate`` for these classes.

    Sandbox mode (R06): rejects NON_IDEMPOTENT_WRITE, AGENT_SPAWN, and
    SENSITIVE before the tool body runs.

    Durable backend hooks default to the DBOS adapter. Pass Restate (or local
    memo) wrappers via ``durable_infer_fn`` / ``durable_tool_fn`` /
    ``durable_workflow_fn`` together for a second backend. Partial injection
    raises ``ValueError`` so backends are never mixed silently.

    ``world_snapshot`` is sealed onto ``DECISION`` (spec §8.4). Pass a
    string-to-string mapping, or ``callable(step_n) -> mapping`` when each
    step has its own facts. A bare mapping is reused for every step. On
    resume, a ``DECISION`` that already exists keeps its sealed snapshot.
    Re-sealing the host's current snapshot would raise ``SlotConflictError``
    before ``check_world`` can report ``WORLD_DRIFT``.
    """
    durable_infer, durable_tool, durable_workflow = _resolve_durable_hooks(
        durable_infer_fn,
        durable_tool_fn,
        durable_workflow_fn,
    )
    run_mode = normalize_run_mode(mode)

    @durable_workflow
    def run_step(step_n: int, model_call, context: dict):
        node_log.append("PROJECT_CONTEXT", step_n, context, trajectory_id, tenant_id, seq=0)

        # Model inference wrapped as a durable step -- fix from spec §1. Without
        # durable_infer here, a crash after this line but before the DECISION
        # is sealed would cause the backend to re-invoke model_call on resume even
        # though its output would be discarded once replay reaches DECISION.
        infer = durable_infer(model_call)
        plan = _require_plan_object(infer(context))

        # append() is idempotent by content, so this doubles as the "seal":
        # replaying it after a crash produces the same node id and is a no-op
        # when the payload matches. A resume that already has this DECISION
        # keeps the sealed snapshot instead of calling world_snapshot again.
        node_log.append(
            "DECISION",
            step_n,
            decision_payload(
                plan,
                _decision_world_snapshot(
                    node_log,
                    trajectory_id,
                    tenant_id,
                    step_n,
                    world_snapshot,
                ),
            ),
            trajectory_id,
            tenant_id,
            seq=1,
        )
        if on_decision_sealed is not None:
            on_decision_sealed()
        # Observe after the seal exists so resume re-checks the world against
        # the original snapshot. Do not re-seal a different payload.
        if observe_world is not None:
            check_world(
                node_log,
                trajectory_id,
                tenant_id,
                step_n,
                observe_world(),
            )

        calls = _tool_calls_from_plan(plan)
        results = []
        for i, (name, args) in enumerate(calls):
            if name not in tool_registry:
                raise ValueError(f'resume: unknown tool "{name}"')
            tool = tool_registry[name]
            # Two slots per tool call: TOOL_CALL at `seq`, its outcome
            # (TOOL_RESULT or ABORT) at `seq + 1`. Stride 1 would make tool i's
            # outcome collide with tool i+1's TOOL_CALL, which breaks seq as a
            # total order *and* breaks the gate: it resolves "was this exact
            # call already attempted" by looking up (step_n, seq), so seq has to
            # identify one call unambiguously.
            seq = 2 + 2 * i
            assert_open_world_effect(
                tool.name,
                tool.effect_class,
                allow_override=tool.allow_open_world_override,
            )
            assert_open_world_effect(
                name,
                tool.effect_class,
                allow_override=tool.allow_open_world_override,
            )
            assert_tool_allowed_in_mode(
                run_mode,
                tool_name=name,
                effect_class=tool.effect_class,
            )
            if requires_block_and_gate(tool.effect_class):
                gated = make_gated_tool_call(
                    node_log,
                    trajectory_id,
                    tenant_id,
                    step_n,
                    seq,
                    name,
                    tool.fn,
                )
                result = durable_tool(gated)(**args)
            else:
                plain = make_plain_tool_call(
                    node_log,
                    trajectory_id,
                    tenant_id,
                    step_n,
                    seq,
                    name,
                    tool.fn,
                )
                result = durable_tool(plain)(**args)
            results.append(result)

        node_log.append(
            "COMMIT_STEP",
            step_n,
            {},
            trajectory_id,
            tenant_id,
            seq=2 + 2 * len(calls),
        )
        return results

    return run_step
