"""Smoke and path coverage for examples/adoption_host (public APIs only)."""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

import pytest

_REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_EXAMPLE = os.path.join(_REPO, "examples", "adoption_host")
if _EXAMPLE not in sys.path:
    sys.path.insert(0, _EXAMPLE)

from examples.adoption_host import run_demo  # noqa: E402

from trajectory_ir.effects import EffectClass  # noqa: E402
from trajectory_ir.package import load_tir  # noqa: E402
from trajectory_ir.runtime.tool import Tool  # noqa: E402
from trajectory_ir.storage import FileSystemCAS  # noqa: E402


def test_live_host_step_pure_and_gated(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "adoption.sqlite")
    step = run_demo.run_host_step(db_path=db, sandbox=False)

    assert step.tool_results == (
        {"service": "api", "release": "1.0.0", "status": "ready"},
        {"shipped": "api:1.0.0"},
    )
    assert "PROJECT_CONTEXT" in step.node_kinds
    assert "DECISION" in step.node_kinds
    assert "TOOL_CALL" in step.node_kinds
    assert "TOOL_RESULT" in step.node_kinds
    assert "COMMIT_STEP" in step.node_kinds
    # PURE tool + gated tool each contribute a TOOL_CALL (2) and TOOL_RESULT (2).
    assert step.node_kinds.count("TOOL_CALL") == 2
    assert step.node_kinds.count("TOOL_RESULT") == 2


def test_sandbox_rejects_non_idempotent(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "sandbox.sqlite")
    with pytest.raises(run_demo.SandboxForbidden):
        run_demo.run_host_step(db_path=db, sandbox=True)


def test_custom_context_and_model(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "custom.sqlite")

    def model(ctx):
        return {
            "tool_calls": [
                {
                    "name": "build_manifest",
                    "args": {
                        "service": ctx["service"],
                        "release": ctx["release"],
                    },
                }
            ]
        }

    step = run_demo.run_host_step(
        db_path=db,
        model_call=model,
        context={"service": "worker", "release": "2.1.0"},
        tools={
            "build_manifest": run_demo.make_tools()["build_manifest"],
        },
    )
    assert step.tool_results == ({"service": "worker", "release": "2.1.0", "status": "ready"},)
    assert step.node_kinds.count("TOOL_CALL") == 1


def test_unknown_tool_raises(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "bad-tool.sqlite")

    def model(_ctx):
        return {"tool_calls": [{"name": "does_not_exist", "args": {}}]}

    with pytest.raises(KeyError, match="unknown tool"):
        run_demo.run_host_step(db_path=db, model_call=model)


def test_invalid_plan_raises(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "bad-plan.sqlite")

    def model(_ctx):
        return {"not_tool_calls": []}

    with pytest.raises(ValueError, match="tool_calls"):
        run_demo.run_host_step(db_path=db, model_call=model)


def test_export_thin_package_roundtrip(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "pkg.sqlite")
    step = run_demo.run_host_step(db_path=db, sandbox=False)
    payload = run_demo.report_bytes_from_step(step)
    assert b"adoption-host-demo" in payload

    cas_root = tmp_path / "cas"
    dest = tmp_path / "run.tir"
    package = run_demo.export_thin_package(
        db_path=db,
        cas_root=cas_root,
        dest=dest,
        payload=payload,
        trajectory_id=step.trajectory_id,
        tenant_id=step.tenant_id,
    )

    assert Path(package.tir_path).is_file()
    assert package.rehydrated_bytes == payload
    assert package.node_count >= 1
    assert package.logical_path == "outputs/release-report.json"

    pkg = load_tir(package.tir_path)
    assert pkg.manifest["mode"] == "thin"
    assert pkg.artifact_bytes == {}
    assert pkg.artifacts_manifest[0]["content_hash"] == package.content_hash
    assert FileSystemCAS(cas_root).has(package.content_hash)


def test_export_rejects_empty_payload(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = str(tmp_path / "empty.sqlite")
    run_demo.run_host_step(db_path=db, sandbox=False)
    with pytest.raises(ValueError, match="non empty"):
        run_demo.export_thin_package(
            db_path=db,
            cas_root=tmp_path / "cas",
            dest=tmp_path / "x.tir",
            payload=b"",
        )


def test_main_live_exit_zero(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    code = run_demo.main(["--db", str(tmp_path / "m.sqlite")])
    assert code == 0


def test_main_sandbox_exit_zero(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    code = run_demo.main(["--sandbox", "--db", str(tmp_path / "s.sqlite")])
    assert code == 0


def test_main_with_package_exit_zero(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    db = tmp_path / "p.sqlite"
    cas = tmp_path / "cas"
    tir = tmp_path / "out.tir"
    code = run_demo.main(
        [
            "--db",
            str(db),
            "--with-package",
            "--cas-root",
            str(cas),
            "--tir-out",
            str(tir),
        ]
    )
    assert code == 0
    assert tir.is_file()
    assert cas.is_dir()
    # CAS root should contain at least one object shard.
    assert any(cas.rglob("*")), "expected CAS objects under cas root"


def test_main_rejects_sandbox_with_package(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    code = run_demo.main(["--sandbox", "--with-package", "--db", str(tmp_path / "x.sqlite")])
    assert code == 2


def test_main_temp_db_and_package_cleanup(tmp_path, monkeypatch):
    """Default paths use temp files; process must still exit 0."""
    monkeypatch.chdir(tmp_path)
    code = run_demo.main(["--with-package"])
    assert code == 0


def test_report_bytes_is_stable_json(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    step = run_demo.run_host_step(db_path=str(tmp_path / "r.sqlite"))
    raw = run_demo.report_bytes_from_step(step)
    parsed = json.loads(raw.decode("utf-8"))
    assert parsed["trajectory_id"] == run_demo.TRAJECTORY_ID
    assert isinstance(parsed["tool_results"], list)
    assert isinstance(parsed["node_kinds"], list)


def test_make_tools_effect_classes():
    tools = run_demo.make_tools()
    assert tools["build_manifest"].effect_class is EffectClass.PURE
    assert tools["ship_release"].effect_class is EffectClass.NON_IDEMPOTENT_WRITE
    assert isinstance(tools["build_manifest"], Tool)
