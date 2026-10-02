"""Observation polarity, full inputs, and frozen independent judge execution."""

import copy
import json
from pathlib import Path

import pytest
from test_evaluation import define, run
from test_evaluation import lab as lab_fixture
from test_evaluation_judges import config, execute

from character_lab import assessments, evaluation, evaluation_judges
from character_lab import evaluation_policies as policies
from character_lab import evaluation_sets as datasets

lab = lab_fixture


@pytest.mark.asyncio
@pytest.mark.parametrize("probability,passes", [(0.95, False), (0.04, True)])
async def test_absence_uses_complement_but_keeps_observed_probability(
    lab, monkeypatch, probability, passes
):
    requests = []

    async def classify(request, trace):
        requests.append(copy.deepcopy(request))
        trace.update(status="complete", request=request, scores={"passes": probability})

    monkeypatch.setattr(evaluation, "classify", classify)
    # The harmful content is early; a last-message-only assessment would miss it.
    text = "visitor: Harmful statement.\n" + "character: Neutral reply.\n" * 500
    node = lab.project.add(text)
    definition = define(
        lab,
        kind="jev",
        model="jev",
        expected="absent",
        spec="Harmful language in the latest message",
        threshold=0.8,
    )
    records = await run(lab, definition, [{"node": node["id"]}], train_on_pass=True)
    record = records[0]
    assert record["result"]["probability"] == probability
    assert record["result"]["desired_probability"] == pytest.approx(1 - probability)
    assert record["passed"] is passes
    assert record["training"] is passes
    assert requests[0]["state"]["text"] == text
    assert "whole supplied item" in requests[0]["questions"]["passes"]["instructions"]
    assert record["definition"]["spec"] == definition["spec"]
    assert record["trace"]["scope"] == "whole_item"
    frozen = copy.deepcopy(record)
    define(lab, **(definition | {"expected": "present"}))
    assert record == frozen


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "observed,expected,passes",
    [
        (True, "absent", False),
        (False, "absent", True),
        (True, "present", True),
    ],
)
async def test_llm_observation_and_evidence_survive_outcome_mapping(
    observed, expected, passes
):
    raw = dict(passed=observed, reason="Observed text", evidence="full document")

    class Judge:
        async def judge(self, messages, trace):
            assert json.loads(messages[1]["content"])["text"] == "The full document."
            assert "Policy acceptance" in messages[0]["content"]
            return raw

    record = dict(
        text="The full document.",
        definition=dict(
            id="harm",
            kind="llm",
            model="judge",
            prompt=assessments.DEFAULT_PROMPT,
            spec="Harmful language",
            expected=expected,
        ),
    )
    (result,) = await assessments.assess_group([record], Judge())
    assert result["passed"] is passes
    assert result["observed"] is observed
    assert result["evidence_start"] == 4
    assert result["evidence_end"] == 17
    assert record["raw_result"] == raw
    assert raw["passed"] is observed


def test_legacy_behavior_defaults_do_not_rewrite_records_or_increment_revisions():
    behavior = dict(
        id="old", name="Old", spec="Voice", threshold=0.8, enabled=True, revision=4
    )
    original = copy.deepcopy(behavior)
    assert evaluation_judges.normalize_behaviors([behavior], [behavior]) == [original]
    assert assessments.expected_state(behavior) == "present"
    assert assessments.outcome(dict(passed=True), behavior)["passed"] is True
    assert behavior == original
    with pytest.raises(ValueError, match="present or absent"):
        evaluation_judges.normalize_behaviors([behavior | {"expected": "maybe"}])


@pytest.mark.asyncio
async def test_disabled_behaviors_do_not_generate_assessments(lab):
    judge = config()
    judge["behaviors"][0].update(expected="absent", enabled=False)
    judge["behaviors"][1]["expected"] = "present"
    records = await execute(lab, [judge])
    assert len(records) == 1
    assert records[0]["definition"]["behavior_id"] == "b1"
    group = datasets.resolve(lab.project)
    assert (
        datasets.item_summary(lab.project, group, group["items"][0])["status"]
        == "complete"
    )


@pytest.mark.asyncio
async def test_distinct_llm_judges_use_frozen_models_and_close_before_switch(
    lab, tmp_path
):
    second_path = tmp_path / "second.gguf"
    second_path.touch()
    second_model = dict(
        name="Second model", alias="second-model", path=str(second_path)
    )
    lab.judge_models.append(second_model)
    first, second = config(), config(name="Second")
    second["model"] = second_model["alias"]
    policies.save(lab.project, dict(name="Two models", judges=[first, second]))
    group, definitions = datasets.plan(lab)
    assert {d["resolved_model"]["alias"] for d in definitions} == {
        "judge",
        "second-model",
    }
    node = lab.project.add("Exact input")
    item = evaluation.capture(lab.project, {"node": node["id"]})
    # prepare expects a collection item; add it through the normal data helper.
    datasets.add_capture(group, item)
    records = datasets.prepare(lab, group, definitions, group["items"])
    lab.judge_models[1] = dict(second_model, path="changed-after-plan.gguf")
    lifecycle = []

    class Judge:
        def __init__(self, folder, model):
            self.model = model
            lifecycle.append(("open", copy.deepcopy(model)))

        async def judge(self, messages, trace):
            return dict(passed=True, reason="Observed", evidence="Exact input")

        def close(self):
            lifecycle.append(("close", self.model["alias"]))

    lab.runtime_factory = Judge
    await evaluation.evaluate(lab, records)
    assert [event[0] for event in lifecycle] == ["open", "close", "open", "close"]
    assert lifecycle[2][1] == second_model
    assert all(r["status"] == "complete" for r in records)
    assert records[-1]["trace"]["resolved_model"] == second_model


@pytest.mark.asyncio
@pytest.mark.parametrize("command", ["continue", "simulator.run", "evaluation.run"])
async def test_missing_judge_weights_rejected_before_queuing_or_capturing(lab, command):
    definition = define(lab)
    policies.save(lab.project, dict(name="Quality", judges=[config()]))
    node = lab.project.add("Seed input")
    if command == "simulator.run":
        node["kept"] = True
        await lab.execute(
            "simulator.configure",
            {
                "documents": [node["id"]],
                "turns": 1,
                "opening_mode": "fixed",
                "opening": "Hello",
            },
            "configure",
        )
    Path(lab.judge_models[0]["path"]).unlink()
    before = copy.deepcopy(lab.project.data)
    args = {"node": node["id"], "eval": "Quality"}
    if command == "evaluation.run":
        args = {"definition": definition["id"], "targets": [{"node": node["id"]}]}
    with pytest.raises(ValueError, match="model file is unavailable"):
        await lab.execute(command, args, "missing-weights")
    assert lab.job is None
    assert lab.project.data == before


@pytest.mark.asyncio
async def test_separate_mode_rejects_multiple_records_in_one_call():
    record = dict(
        text="Input", definition=dict(id="a", kind="llm", call_mode="separate")
    )
    with pytest.raises(ValueError, match="one behavior per call"):
        await assessments.assess_group([record, copy.deepcopy(record)], None)
