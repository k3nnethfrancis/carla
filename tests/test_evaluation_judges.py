"""Actual request grouping and frozen hierarchy without provider inference."""

import asyncio
import copy
import json

import pytest
from test_evaluation import lab as lab_fixture

from character_lab import (
    evaluation,
)
from character_lab import (
    evaluation_policies as policies,
)
from character_lab import (
    evaluation_sets as data,
)

lab = lab_fixture


def config(mode="separate", kind="llm", name="Reader"):
    return dict(
        id=name,
        name=name,
        kind=kind,
        model="judge",
        prompt=evaluation.DEFAULT_PROMPT,
        call_mode=mode,
        behaviors=[
            dict(
                id=f"b{i}",
                name=f"Behavior {i}",
                spec=f"Criteria {i}",
                threshold=0.8,
                enabled=True,
            )
            for i in range(2)
        ],
    )


async def execute(lab, judges):
    policies.save(lab.project, dict(name="Quality", judges=judges))
    await lab.execute(
        "evaluation.collection.save", {"name": "Explicit test data"}, "data"
    )
    dataset = data.resolve(lab.project)
    node = lab.project.add("Exact source text")
    await lab.execute(
        "evaluation.collection.run",
        {"collection": dataset["id"], "targets": [{"node": node["id"]}]},
        "test",
    )
    await lab.job
    return lab.project.data["evaluations"]


@pytest.mark.asyncio
@pytest.mark.parametrize("mode,expected", [("separate", 4), ("bundled", 2)])
async def test_llm_grouping_and_judge_isolation(lab, mode, expected):
    calls = []

    class Judge:
        def __init__(self, *args):
            pass

        def close(self):
            pass

        async def judge(self, messages, trace):
            calls.append(messages)
            payload = json.loads(messages[1]["content"])
            value = dict(passed=True, reason="Matches", evidence=payload["text"])
            return (
                {"results": {b["id"]: value for b in payload["behaviors"]}}
                if "behaviors" in payload
                else value
            )

    lab.runtime_factory = Judge
    records = await execute(lab, [config(mode), config(mode, name="Second")])
    assert len(calls) == expected
    assert len({r["definition"]["id"] for r in records}) == 4
    assert all(r["status"] == "complete" for r in records)
    run = lab.project.data["evaluation_runs"][0]
    assert len(run["policy"]["behaviors"]) == 2
    assert "model" not in run["policy"]["behaviors"][0]
    frozen = copy.deepcopy(run)
    policy = policies.active(lab.project)
    changed = copy.deepcopy(policy)
    changed["judges"][0]["prompt"] += " Different instructions."
    policies.save(lab.project, changed)
    assert run == frozen
    group = data.resolve(lab.project)
    assert (
        data.item_summary(lab.project, group, group["items"][0])["status"] == "evidence"
    )


@pytest.mark.asyncio
@pytest.mark.parametrize("mode,expected", [("separate", 2), ("bundled", 1)])
async def test_classifier_independent_probabilities(lab, monkeypatch, mode, expected):
    calls = []

    async def classify(request, trace, **kwargs):
        calls.append(request)
        trace.update(
            status="complete",
            request=request,
            scores={k: 0.9 for k in request["questions"]},
        )

    monkeypatch.setattr(evaluation, "classify", classify)
    records = await execute(lab, [config(mode, "jev")])
    assert len(calls) == expected
    assert [r["result"]["probability"] for r in records] == [0.9, 0.9]


@pytest.mark.asyncio
@pytest.mark.parametrize("bad", ["missing", "extra", "quote"])
async def test_bundled_invalid_output_fails_whole_call(lab, bad):
    class Judge:
        def __init__(self, *args):
            pass

        def close(self):
            pass

        async def judge(self, messages, trace):
            p = json.loads(messages[1]["content"])
            values = {
                b["id"]: dict(
                    passed=True,
                    reason="Matches",
                    evidence="invented" if bad == "quote" else p["text"],
                )
                for b in p["behaviors"]
            }
            if bad == "missing":
                values.pop(next(iter(values)))
            if bad == "extra":
                values["unknown"] = next(iter(values.values()))
            return {"results": values}

    lab.runtime_factory = Judge
    records = await execute(lab, [config("bundled")])
    assert all(r["status"] == "failed" and "raw_result" in r for r in records)


@pytest.mark.asyncio
async def test_cancel_bundle_marks_all_and_skips_disabled(lab):
    entered = asyncio.Event()

    class Judge:
        def __init__(self, *args):
            pass

        def close(self):
            pass

        async def judge(self, messages, trace):
            entered.set()
            await asyncio.Event().wait()

    lab.runtime_factory = Judge
    judge = config("bundled")
    judge["behaviors"].append(
        dict(
            id="disabled",
            name="Disabled",
            spec="Not used",
            threshold=0.5,
            enabled=False,
        )
    )
    policies.save(lab.project, dict(name="Quality", judges=[judge]))
    node = lab.project.add("Some text")
    await lab.execute(
        "evaluation.collection.run", {"targets": [{"node": node["id"]}]}, "test"
    )
    await entered.wait()
    task = lab.job
    task.cancel()
    await task
    records = lab.project.data["evaluations"]
    assert len(records) == 2
    assert all(r["status"] == "stopped" and "passed" not in r for r in records)


@pytest.mark.asyncio
async def test_mixed_judges_two_items_never_bundle_across_inputs(lab, monkeypatch):
    llm_calls, classifier_calls = [], []

    class Judge:
        def __init__(self, *args):
            pass

        def close(self):
            pass

        async def judge(self, messages, trace):
            payload = json.loads(messages[1]["content"])
            llm_calls.append(payload)
            return {
                "results": {
                    b["id"]: dict(
                        passed=True, reason="Matches", evidence=payload["text"]
                    )
                    for b in payload["behaviors"]
                }
            }

    async def classify(request, trace, **kwargs):
        classifier_calls.append(request)
        trace.update(status="complete", scores={k: 0.9 for k in request["questions"]})

    lab.runtime_factory = Judge
    monkeypatch.setattr(evaluation, "classify", classify)
    policies.save(
        lab.project,
        dict(
            name="Mixed",
            judges=[config("bundled"), config("bundled", "jev", "Classifier")],
        ),
    )
    nodes = [lab.project.add(t) for t in ("First exact input", "Second exact input")]
    await lab.execute(
        "evaluation.collection.run",
        {"targets": [{"node": n["id"]} for n in nodes]},
        "test",
    )
    await lab.job
    assert [p["text"] for p in llm_calls] == [n["text"] for n in nodes]
    assert [p["state"]["text"] for p in classifier_calls] == [n["text"] for n in nodes]
    records = lab.project.data["evaluations"]
    assert len(records) == 8 and all(r["status"] == "complete" for r in records)


def test_revision_stability_and_private_policy_copies(lab):
    first = policies.save(lab.project, dict(name="First", judges=[config()]))
    second = policies.save(
        lab.project,
        dict(name="Second", judges=first["judges"], behaviors=first["behaviors"]),
    )
    before = copy.deepcopy(second)
    unchanged = copy.deepcopy(first["judges"])
    policies.save(lab.project, copy.deepcopy(first))
    assert first["judges"] == unchanged
    changed = copy.deepcopy(first)
    changed["behaviors"][0]["spec"] = "New criteria"
    policies.save(lab.project, changed)
    assert first["judges"][0]["revision"] == 1
    assert first["behaviors"][0]["revision"] == 2
    assert second == before


@pytest.mark.asyncio
async def test_copied_policy_same_ids_and_revisions_do_not_reuse_different_specs(lab):
    await execute(lab, [config()])
    first = policies.active(lab.project)
    fork = copy.deepcopy(first)
    fork.pop("id")
    fork["name"] = "Different requirements"
    fork["behaviors"][0]["spec"] = "Different criterion"
    second = policies.save(lab.project, fork)
    assert policies.active(lab.project)["id"] == first["id"]
    lab.project.data["active_evaluation_policy"] = second["id"]
    assert first["judges"][0]["revision"] == second["judges"][0]["revision"]
    group = data.resolve(lab.project)
    assert (
        data.item_summary(lab.project, group, group["items"][0])["status"] == "evidence"
    )


@pytest.mark.asyncio
async def test_flat_migration_preserves_variants_revisions_and_frozen_runs(lab):
    await execute(lab, [config()])
    records = copy.deepcopy(lab.project.data["evaluations"])
    nested = config()
    nested["revision"] = 5
    nested["behaviors"][0].update(revision=7, source_id="library", source_revision=3)
    nested["behaviors"][1]["revision"] = 2
    identical = copy.deepcopy(nested)
    identical.update(id="second", name="Second")
    conflicting = copy.deepcopy(nested)
    conflicting.update(id="third", name="Third")
    conflicting["behaviors"][0]["spec"] = "A different requirement"
    policy = policies.active(lab.project)
    policy.pop("behaviors")
    policy["judges"] = [nested, identical, conflicting]
    # A historical envelope keeps exactly its old nested representation.
    lab.project.data["evaluation_runs"][0]["policy"] = copy.deepcopy(policy)
    history = copy.deepcopy(lab.project.data["evaluation_runs"])
    policies.migrate(lab.project)
    assert lab.project.data["evaluations"] == records
    assert lab.project.data["evaluation_runs"] == history
    assert all("behaviors" not in j for j in policy["judges"])
    assert len(policy["behaviors"]) == 3
    original, second, variant = policy["behaviors"]
    assert original["id"] == "b0" and original["revision"] == 7
    assert original["source_revision"] == 3
    assert variant["id"] != "b0" and "Third" in variant["name"]
    assert variant["spec"] == "A different requirement"
    assert variant["revision"] == 7
    from character_lab import evaluation_judges

    assert (
        evaluation_judges.normalize_behaviors(policy["behaviors"], policy["behaviors"])
        == policy["behaviors"]
    )
    assert second["revision"] == 2
    frozen = copy.deepcopy(lab.project.data)
    policies.migrate(lab.project)
    assert lab.project.data == frozen


@pytest.mark.asyncio
async def test_flat_policy_behavior_edit_reaches_every_judge_next_run(lab, monkeypatch):
    from character_lab import evaluation_judges

    calls = []

    async def classify(request, trace, **kwargs):
        calls.append(request)
        trace.update(status="complete", scores={k: 0.9 for k in request["questions"]})

    monkeypatch.setattr(evaluation, "classify", classify)
    first = config("bundled", "jev")
    behaviors = first.pop("behaviors")
    second = copy.deepcopy(first)
    second.update(id="other", name="Other")
    policy = policies.save(
        lab.project, dict(name="Shared", judges=[first, second], behaviors=behaviors)
    )
    old = copy.deepcopy(policy)
    node = lab.project.add("Source text")
    await lab.execute(
        "evaluation.collection.run", dict(targets=[{"node": node["id"]}]), "first"
    )
    await lab.job
    changed = copy.deepcopy(policy)
    changed["behaviors"][0]["spec"] = "New shared criteria"
    policies.save(lab.project, changed)
    await lab.execute(
        "evaluation.collection.run", dict(targets=[{"node": node["id"]}]), "second"
    )
    await lab.job
    assert len(calls) == 4
    assert all(
        "New shared criteria" in next(iter(c["questions"].values()))["instructions"]
        for c in calls[2:]
    )
    assert lab.project.data["evaluation_runs"][0]["policy"] == old
    assert {d["id"] for d in evaluation_judges.flatten(policy)} == {
        "Reader:b0",
        "Reader:b1",
        "other:b0",
        "other:b1",
    }


def test_migration_keeps_distinct_behavior_ids_with_identical_content():
    from character_lab import evaluation_judges

    first = config()
    first["revision"] = 1
    first["behaviors"] = [
        dict(
            id="first-spec",
            name="Voice",
            spec="Consistent voice",
            enabled=True,
            threshold=0.8,
            revision=4,
        )
    ]
    second = copy.deepcopy(first)
    second.update(id="second-judge", name="Second")
    second["behaviors"][0]["id"] = "second-spec"
    policy = dict(judges=[first, second])
    evaluation_judges.lift_behaviors(policy)
    assert [b["id"] for b in policy["behaviors"]] == ["first-spec", "second-spec"]
    assert all(b["revision"] == 4 for b in policy["behaviors"])
    assert {d["id"] for d in evaluation_judges.flatten(policy)} == {
        "Reader:first-spec",
        "Reader:second-spec",
        "second-judge:first-spec",
        "second-judge:second-spec",
    }
    unchanged = evaluation_judges.normalize_behaviors(
        policy["behaviors"], policy["behaviors"]
    )
    assert unchanged == policy["behaviors"]


def test_migration_deduplicates_repeated_conflicting_original_variant():
    from character_lab import evaluation_judges

    first = config()
    first["revision"] = 1
    first["behaviors"] = [
        dict(
            id="same",
            name="Behavior",
            spec="A",
            enabled=True,
            threshold=0.8,
            revision=3,
            source_id="source",
            source_revision=2,
        )
    ]
    second = copy.deepcopy(first)
    second.update(id="second", name="Second")
    second["behaviors"][0]["spec"] = "B"
    third = copy.deepcopy(second)
    third.update(id="third", name="Third")
    policy = dict(judges=[first, second, third])
    evaluation_judges.lift_behaviors(policy)
    assert len(policy["behaviors"]) == 2
    assert [b["spec"] for b in policy["behaviors"]] == ["A", "B"]
    assert len(evaluation_judges.flatten(policy)) == 6
    before = copy.deepcopy(policy)
    assert evaluation_judges.lift_behaviors(policy) is False
    assert policy == before


@pytest.mark.parametrize(
    "field,value",
    [("revision", 4), ("source_revision", 3), ("threshold", 0.9), ("enabled", False)],
)
def test_conflicting_variants_compare_all_original_fields(field, value):
    from character_lab import evaluation_judges

    first = config()
    first["behaviors"] = [
        dict(
            id="same",
            name="Behavior",
            spec="A",
            enabled=True,
            threshold=0.8,
            revision=3,
            source_id="source",
            source_revision=2,
        )
    ]
    second = copy.deepcopy(first)
    second.update(id="second", name="Second")
    second["behaviors"][0][field] = value
    policy = dict(judges=[first, second])
    evaluation_judges.lift_behaviors(policy)
    assert len(policy["behaviors"]) == 2
    assert policy["behaviors"][1][field] == value
