"""Data/policy independence and immutable run provenance, using fake inference."""

import copy

import pytest
from test_evaluation import define, run
from test_evaluation import lab as lab_fixture

from character_lab import evaluation_policies as policies
from character_lab import evaluation_sets as data

lab = lab_fixture


@pytest.mark.asyncio
async def test_policy_runs_independently_of_data_and_freezes_configuration(lab):
    first = define(lab)
    second = define(lab, name="Voice", spec="Voice stays consistent")
    await lab.execute(
        "evaluation.policy.save",
        {"name": "Quality", "judges": [first["id"], second["id"]]},
        "policy",
    )
    policy = policies.active(lab.project)
    await lab.execute("evaluation.collection.save", {"name": "Samples"}, "data")
    group = data.resolve(lab.project)
    node = lab.project.add("A coherent continuation")
    await lab.execute(
        "evaluation.collection.run",
        {"policy": "Quality", "targets": [{"node": node["id"]}]},
        "run",
    )
    await lab.job
    result = policies.run_summaries(lab.project)[0]
    assert result["status"] == "complete" and result["passed"] is True
    assert len(result["records"]) == 2
    assert result["collection"] == group["id"]
    frozen = copy.deepcopy(result)
    define(lab, id=first["id"], spec="A changed spec")
    await lab.execute(
        "evaluation.policy.save",
        {"id": policy["id"], "name": "Changed", "judges": [second["id"]]},
        "edit",
    )
    assert policies.run_summaries(lab.project)[0] == frozen
    await lab.execute("evaluation.run.open", {"id": result["id"]}, "open")
    assert lab.events[-1][0] == "evaluation_run"
    assert len(lab.events[-1][1]["results"]) == 2
    assert lab.events[-1][1]["policy"]["name"] == "Quality"
    await lab.execute("evaluation.policy.delete", {"id": policy["id"]}, "delete")
    assert policies.run_summaries(lab.project)[0] == frozen
    assert group["items"][0]["text"] == "A coherent continuation"


@pytest.mark.asyncio
async def test_legacy_split_preserves_records_and_missing_behavior_references(lab):
    definition = define(lab)
    node = lab.project.add("Old trace")
    await run(lab, definition, [{"node": node["id"]}])
    records = copy.deepcopy(lab.project.data["evaluations"])
    group = dict(
        id="legacy", name="Legacy", judges=[definition["id"], "deleted"], items=[]
    )
    lab.project.data["evaluation_sets"] = [group]
    lab.project.data["active_evaluation"] = "legacy"
    del lab.project.data["evaluation_policies"]
    del lab.project.data["evaluation_runs"]
    data.migrate(lab.project)
    assert lab.project.data["evaluations"] == records
    assert [j["id"] for j in policies.active(lab.project)["judges"]] == [
        definition["id"],
        "deleted",
    ]
    assert (
        policies.active(lab.project)["judges"][0]["behaviors"][0]["spec"]
        == definition["spec"]
    )
    assert policies.active(lab.project)["judges"][1]["missing"]
    assert len(data.collections(lab.project)) == 2
    assert data.collections(lab.project)[1]["items"][0]["text"] == "Old trace"
    assert policies.run_summaries(lab.project)[0]["passed"] is True
    state = copy.deepcopy(lab.project.data)
    data.migrate(lab.project)
    assert lab.project.data == state


@pytest.mark.asyncio
async def test_generation_plan_freezes_policy_and_creates_default_data(
    lab, monkeypatch
):
    from test_evaluation import Judge
    from test_service import FakeRuntime

    monkeypatch.setattr(Judge, "stream", FakeRuntime.stream, raising=False)
    definition = define(lab)
    await lab.execute(
        "evaluation.policy.save",
        {"name": "Quality", "judges": [definition["id"]]},
        "policy",
    )
    node = lab.project.add("Seed")
    await lab.execute("continue", {"node": node["id"], "eval": "Quality"}, "loom")
    await lab.job
    assert data.collections(lab.project)[0]["name"] == "Data"
    result = policies.run_summaries(lab.project)[0]
    assert result["policy"]["name"] == "Quality"
    assert result["passed"] is True
    with pytest.raises(ValueError, match="policies"):
        await lab.execute(
            "evaluation.definition.delete", {"id": definition["id"]}, "delete"
        )


@pytest.mark.asyncio
async def test_editing_inactive_policy_preserves_active_and_old_empty_data_recovers(
    lab,
):
    behavior = define(lab)
    first = policies.save(lab.project, {"name": "First", "judges": [behavior["id"]]})
    second = policies.save(lab.project, {"name": "Second", "judges": [behavior["id"]]})
    assert policies.active(lab.project)["id"] == first["id"]
    lab.project.data["active_evaluation_policy"] = second["id"]
    policies.save(
        lab.project, {"id": first["id"], "name": "Edited", "judges": [behavior["id"]]}
    )
    assert policies.active(lab.project)["id"] == second["id"]
    node = lab.project.add("Legacy standalone result")
    await run(lab, behavior, [{"node": node["id"]}])
    lab.project.data["evaluation_sets"] = []
    lab.project.data["active_evaluation"] = ""
    del lab.project.data["evaluation_runs"]
    policies.migrate(lab.project)
    assert data.resolve(lab.project)["name"] == "Historical data"
    assert data.resolve(lab.project)["items"][0]["text"] == "Legacy standalone result"
