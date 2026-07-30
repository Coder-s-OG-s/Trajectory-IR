from drivers.durable_backend.dbos.adapter import durable_infer, durable_tool, durable_workflow, init_backend


def test_wrapped_workflow_runs_and_returns_result(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    init_backend(app_name="test-adapter")

    call_count = {"n": 0}

    def model_call(x):
        call_count["n"] += 1
        return x * 2

    infer = durable_infer(model_call)

    @durable_workflow
    def workflow(x):
        return infer(x)

    result = workflow(5)
    assert result == 10
    assert call_count["n"] == 1
