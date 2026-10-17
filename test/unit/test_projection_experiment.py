from integrations.projection_eval.experiment import run_experiment


def test_small_arm_keeps_the_answer_and_does_not_invent_provider_tokens() -> None:
    result = run_experiment()
    full, small = result["arms"]
    assert full["task_score"] == 1
    assert small["task_score"] == 1
    assert small["char_len"] < full["char_len"]
    assert small["console_estimate"] < full["console_estimate"]
    assert small["provider_prompt_tokens"] is None
    assert result["winner_on_provider_tokens"] is None
    assert small["dropped"] > 0
