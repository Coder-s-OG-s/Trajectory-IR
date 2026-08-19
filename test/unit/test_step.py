from dbos import SetWorkflowID

from drivers.durable_backend.dbos.adapter import init_backend
from trajectory_ir.effects import EffectClass
from trajectory_ir.resume.step import make_run_step
from trajectory_ir.runtime.log import NodeLog
from trajectory_ir.runtime.tool import Tool

TRAJECTORY_ID = "test-step-plain-tool"
TENANT_ID = "demo"


def _echo(msg):
    return msg


def _model_call(context):
    return {"tool_calls": [{"name": "echo", "args": {"msg": "hello"}}]}


def test_plain_tool_logs_tool_call_and_result(tmp_path, monkeypatch):
    """Issue #45: PURE/READ_ONLY/IDEMPOTENT_WRITE tools must still append
    TOOL_CALL/TOOL_RESULT nodes, matching Go's RunStep. Only the
    NON_IDEMPOTENT_WRITE gate path is exempt from needing extra logging
    because make_gated_tool_call already logs both nodes itself."""
    monkeypatch.chdir(tmp_path)

    node_log = NodeLog(str(tmp_path / "nodes.sqlite"))

    tool_registry = {
        "echo": Tool(
            name="echo",
            fn=_echo,
            effect_class=EffectClass.PURE,
        ),
    }

    run_step = make_run_step(node_log, TENANT_ID, TRAJECTORY_ID, tool_registry)

    init_backend(app_name="test-step-plain-tool")

    with SetWorkflowID(TRAJECTORY_ID):
        results = run_step(step_n=1, model_call=_model_call, context={})

    assert results == ["hello"]
    assert node_log.has(TRAJECTORY_ID, TENANT_ID, 1, "TOOL_CALL", seq=2)
    assert node_log.has(TRAJECTORY_ID, TENANT_ID, 1, "TOOL_RESULT", seq=3)
