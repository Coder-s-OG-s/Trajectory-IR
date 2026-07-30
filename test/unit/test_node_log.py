import os
import tempfile

import pytest

from trajectory_ir.runtime.log import NodeLog


@pytest.fixture
def log():
    fd, path = tempfile.mkstemp(suffix=".sqlite")
    os.close(fd)
    node_log = NodeLog(path)
    yield node_log
    node_log.close()
    os.remove(path)


def test_append_then_has(log):
    log.append("DECISION", step_n=1, payload={"plan": "x"}, trajectory_id="t1", tenant_id="demo", seq=1)
    assert log.has("t1", 1, "DECISION")
    assert not log.has("t1", 1, "TOOL_RESULT")


def test_append_is_idempotent_by_content(log):
    n1 = log.append("DECISION", step_n=1, payload={"plan": "x"}, trajectory_id="t1", tenant_id="demo", seq=1)
    n2 = log.append("DECISION", step_n=1, payload={"plan": "x"}, trajectory_id="t1", tenant_id="demo", seq=1)
    assert n1.id == n2.id
    assert log.count(n1.id) == 1
