"""Adoption oriented host demo using only the public Trajectory IR APIs.

This is the example a newcomer should run after ``pip install -e ".[dev]"``
to see how a real host process wires the public client, tools, optional CAS,
and a thin ``.tir`` package. It is not an agent framework and does not call
paid model APIs.

Run from the repository root::

    python examples/adoption_host/run_demo.py
    python examples/adoption_host/run_demo.py --sandbox
    python examples/adoption_host/run_demo.py --with-package

Compared with ``examples/host_loop``: same public client surface, but this
demo also shows optional ``FileSystemCAS`` + thin ``export_tir`` + rehydrate.
Crash safe durable workflow demos remain under ``examples/kill-mid-deploy/``.
"""

from __future__ import annotations

import argparse
import contextlib
import json
import os
import sys
import tempfile
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any

# Allow ``python examples/adoption_host/run_demo.py`` without a prior editable install.
_REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
for _path in (_REPO_ROOT, os.path.join(_REPO_ROOT, "pkg")):
    if _path not in sys.path:
        sys.path.insert(0, _path)

from client.python.trajectory_client import (  # noqa: E402
    commit_step,
    exec_tool,
    open_trajectory,
    project,
    seal_decision,
)
from trajectory_ir.effects import EffectClass  # noqa: E402
from trajectory_ir.package import export_tir, load_tir  # noqa: E402
from trajectory_ir.runtime.log import NodeLog  # noqa: E402
from trajectory_ir.runtime.sandbox import SandboxForbidden  # noqa: E402
from trajectory_ir.runtime.tool import Tool  # noqa: E402
from trajectory_ir.storage import FileSystemCAS, put_artifact, rehydrate_artifacts  # noqa: E402

TENANT_ID = "demo"
TRAJECTORY_ID = "adoption-host-demo"
STEP_N = 1

ModelFn = Callable[[Mapping[str, Any]], dict[str, Any]]


@dataclass(frozen=True)
class HostStepResult:
    """Outcome of one host owned seal / tool / commit cycle."""

    tool_results: tuple[Any, ...]
    node_kinds: tuple[str, ...]
    trajectory_id: str
    tenant_id: str
    db_path: str


@dataclass(frozen=True)
class PackageResult:
    """Outcome of optional thin package + CAS rehydrate check."""

    tir_path: str
    content_hash: str
    logical_path: str
    rehydrated_bytes: bytes
    node_count: int


def stub_model(context: Mapping[str, Any]) -> dict[str, Any]:
    """Deterministic stand in for an LLM. Returns concrete known args only."""
    release = str(context.get("release", "1.0.0"))
    service = str(context.get("service", "api"))
    return {
        "tool_calls": [
            {
                "name": "build_manifest",
                "args": {"service": service, "release": release},
            },
            {
                "name": "ship_release",
                "args": {"service": service, "version": release},
            },
        ]
    }


def make_tools() -> dict[str, Tool]:
    """Tools used by the demo: one PURE path and one gated write path."""

    def build_manifest(*, service: str, release: str) -> dict[str, str]:
        return {
            "service": service,
            "release": release,
            "status": "ready",
        }

    def ship_release(*, service: str, version: str) -> dict[str, str]:
        # Side effect stand in (deploy). Gated as NON_IDEMPOTENT_WRITE.
        return {"shipped": f"{service}:{version}"}

    return {
        "build_manifest": Tool(
            name="build_manifest",
            fn=build_manifest,
            effect_class=EffectClass.PURE,
        ),
        "ship_release": Tool(
            name="ship_release",
            fn=ship_release,
            effect_class=EffectClass.NON_IDEMPOTENT_WRITE,
        ),
    }


def _list_kinds(db_path: str, *, trajectory_id: str, tenant_id: str) -> tuple[str, ...]:
    log = NodeLog(db_path)
    try:
        nodes = log.list_nodes(trajectory_id, tenant_id=tenant_id)
        return tuple(n["kind"] for n in nodes)
    finally:
        log.close()


def run_host_step(
    *,
    db_path: str,
    model_call: ModelFn = stub_model,
    sandbox: bool = False,
    context: Mapping[str, Any] | None = None,
    tools: Mapping[str, Tool] | None = None,
    trajectory_id: str = TRAJECTORY_ID,
    tenant_id: str = TENANT_ID,
    step_n: int = STEP_N,
) -> HostStepResult:
    """Run one host owned step with the public client API only.

    Sequence matches the master README host loop:
    open → project → model → seal_decision → exec_tool* → commit_step.
    """
    mode = "sandbox" if sandbox else "live"
    ctx: dict[str, Any] = {
        "goal": "build a release manifest then ship it",
        "service": "api",
        "release": "1.0.0",
    }
    if context:
        ctx.update(dict(context))

    traj = open_trajectory(
        tenant_id,
        trajectory_id,
        db_path=db_path,
        mode=mode,
    )
    project(traj, step_n=step_n, context=ctx)

    plan = model_call(ctx)
    if "tool_calls" not in plan or not isinstance(plan["tool_calls"], Sequence):
        raise ValueError("model plan must include a tool_calls sequence")
    seal_decision(traj, step_n=step_n, plan=plan)

    registry = dict(tools) if tools is not None else make_tools()
    results: list[Any] = []
    for i, call in enumerate(plan["tool_calls"]):
        name = call["name"]
        if name not in registry:
            raise KeyError(f"unknown tool in sealed plan: {name!r}")
        tool = registry[name]
        # seq = 2 + 2*i keeps TOOL_CALL and TOOL_RESULT slots non overlapping.
        seq = 2 + 2 * i
        tool_result = exec_tool(
            traj,
            step_n=step_n,
            call={"args": call["args"]},
            tool=tool,
            seq=seq,
        )
        results.append(tool_result.result)

    commit_seq = 2 + 2 * len(plan["tool_calls"])
    commit_step(traj, step_n=step_n, seq=commit_seq)

    kinds = _list_kinds(db_path, trajectory_id=trajectory_id, tenant_id=tenant_id)
    return HostStepResult(
        tool_results=tuple(results),
        node_kinds=kinds,
        trajectory_id=trajectory_id,
        tenant_id=tenant_id,
        db_path=db_path,
    )


def export_thin_package(
    db_path: str,
    cas_root: str | Path,
    dest: str | Path,
    payload: bytes,
    *,
    trajectory_id: str = TRAJECTORY_ID,
    tenant_id: str = TENANT_ID,
    logical_path: str = "outputs/release-report.json",
) -> PackageResult:
    """Put ``payload`` into a filesystem CAS, export a thin ``.tir``, rehydrate.

    Uses only public storage and package APIs. The host owns artifact bytes;
    the IR log stays content addressed and independent of the CAS backend.
    """
    if not payload:
        raise ValueError("payload must be non empty")

    store = FileSystemCAS(cas_root)
    ref = put_artifact(store, payload, logical_path=logical_path)

    log = NodeLog(db_path)
    try:
        tir_path = export_tir(
            log,
            trajectory_id,
            dest,
            mode="thin",
            artifacts=[ref],
            tenant_id=tenant_id,
            cas=store,
        )
    finally:
        log.close()

    pkg = load_tir(tir_path)
    rehydrated = rehydrate_artifacts(store, pkg.artifacts_manifest)  # type: ignore[arg-type]
    if ref.content_hash not in rehydrated:
        raise RuntimeError(f"rehydrate missed content_hash={ref.content_hash}")
    if rehydrated[ref.content_hash] != payload:
        raise RuntimeError("rehydrated bytes do not match original payload")

    return PackageResult(
        tir_path=str(tir_path),
        content_hash=ref.content_hash,
        logical_path=logical_path,
        rehydrated_bytes=rehydrated[ref.content_hash],
        node_count=int(pkg.manifest["node_count"]),
    )


def report_bytes_from_step(step: HostStepResult) -> bytes:
    """Serialize tool results into a small host owned artifact payload."""
    body = {
        "trajectory_id": step.trajectory_id,
        "tenant_id": step.tenant_id,
        "tool_results": list(step.tool_results),
        "node_kinds": list(step.node_kinds),
    }
    return (json.dumps(body, sort_keys=True, indent=2) + "\n").encode("utf-8")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Adoption host demo for Trajectory IR (public client + optional CAS)"
    )
    parser.add_argument(
        "--sandbox",
        action="store_true",
        help="Open trajectory in sandbox mode (rejects NON_IDEMPOTENT_WRITE)",
    )
    parser.add_argument(
        "--with-package",
        action="store_true",
        help="After a live step, put a report in FileSystemCAS and export a thin .tir",
    )
    parser.add_argument(
        "--db",
        default="",
        help="SQLite path for the IR log (default: temp file)",
    )
    parser.add_argument(
        "--cas-root",
        default="",
        help="Filesystem CAS root when using --with-package (default: temp dir)",
    )
    parser.add_argument(
        "--tir-out",
        default="",
        help="Destination .tir path when using --with-package (default: temp file)",
    )
    args = parser.parse_args(argv)

    if args.sandbox and args.with_package:
        print(
            "error: --sandbox and --with-package cannot be combined "
            "(sandbox never finishes a live gated step)",
            file=sys.stderr,
        )
        return 2

    cleanup_db = False
    if args.db:
        db_path = args.db
    else:
        fd, db_path = tempfile.mkstemp(suffix=".sqlite")
        os.close(fd)
        cleanup_db = True

    cleanup_cas = False
    cleanup_tir = False
    cas_root = args.cas_root
    tir_out = args.tir_out

    try:
        if args.sandbox:
            try:
                run_host_step(db_path=db_path, sandbox=True)
            except SandboxForbidden as exc:
                print(f"sandbox correctly rejected non idempotent tool: {exc}")
                return 0
            print("error: sandbox should have rejected ship_release", file=sys.stderr)
            return 1

        step = run_host_step(db_path=db_path, sandbox=False)
        print("adoption host step complete")
        print(f"  tool results: {list(step.tool_results)}")
        print(f"  node kinds:   {list(step.node_kinds)}")

        if args.with_package:
            if not cas_root:
                cas_root = tempfile.mkdtemp(prefix="trajir-cas-")
                cleanup_cas = True
            if not tir_out:
                fd, tir_out = tempfile.mkstemp(suffix=".tir")
                os.close(fd)
                cleanup_tir = True
            package = export_thin_package(
                db_path=db_path,
                cas_root=cas_root,
                dest=tir_out,
                payload=report_bytes_from_step(step),
                trajectory_id=step.trajectory_id,
                tenant_id=step.tenant_id,
            )
            print("thin package path complete")
            print(f"  tir:          {package.tir_path}")
            print(f"  content_hash: {package.content_hash}")
            print(f"  logical_path: {package.logical_path}")
            print(f"  node_count:   {package.node_count}")
            print(f"  rehydrate:    {len(package.rehydrated_bytes)} bytes ok")

        return 0
    finally:
        if cleanup_db:
            with contextlib.suppress(OSError):
                os.remove(db_path)
        if cleanup_tir and tir_out:
            with contextlib.suppress(OSError):
                os.remove(tir_out)
        if cleanup_cas and cas_root:
            # Best effort: leave CAS contents if remove fails mid walk.
            with contextlib.suppress(OSError):
                for root, dirs, files in os.walk(cas_root, topdown=False):
                    for name in files:
                        os.remove(os.path.join(root, name))
                    for name in dirs:
                        os.rmdir(os.path.join(root, name))
                os.rmdir(cas_root)


if __name__ == "__main__":
    raise SystemExit(main())
