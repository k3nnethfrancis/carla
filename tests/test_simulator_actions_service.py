"""End-to-end action dispatch, selection loops and frozen evaluation evidence."""

import copy
import json

import pytest
from test_service import FakeRuntime
from test_service import session as session_fixture

from character_lab import evaluation

session = session_fixture


async def configure(s):
    doc = s.project.add("A synthetic anthology.", kind="source")
    doc["kept"] = True
    await s.execute(
        "simulator.configure", {"documents": [doc["id"]], "turns": 1}, "config"
    )


@pytest.mark.asyncio
async def test_session_group_loops_select_entire_alternatives(session):
    s = session
    await configure(s)
    await s.execute("simulator.run", {"action": "loom", "count": 4}, "first")
    await s.job
    original = s.project.data["simulation_runs"][0]
    before = copy.deepcopy(original)
    await s.execute(
        "simulator.run",
        {
            "action": "loom",
            "run": original["id"],
            "count": 2,
            "loops": 2,
        },
        "group-loops",
    )
    await s.job
    runs = s.project.data["simulation_runs"]
    assert len(runs) == 5
    assert original == before
    assert [len(r["conversations"]) for r in runs] == [4] * 5
    assert runs[1]["alternative_group"] == runs[2]["alternative_group"]
    assert runs[3]["alternative_group"] == runs[4]["alternative_group"]
    assert runs[3]["alternative_group"] != runs[1]["alternative_group"]
    assert all(r["parent"]["run"] == runs[1]["id"] for r in runs[3:])
    assert all(len(c["turns"]) == 6 for r in runs[3:] for c in r["conversations"])
    policy = s.project.data["policy_runs"][-1]
    assert policy["status"] == "complete"
    for step in policy["steps"]:
        assert len(step["candidates"]) == 2
        candidates = json.loads(step["messages"][1]["content"])["candidates"]
        assert all("conversation 4" in c["continuation"] for c in candidates)
    assert not s.busy
    assert not [data for kind, data, _ in s.events if kind == "error"]


@pytest.mark.asyncio
async def test_continue_evaluates_only_changed_heads_preserving_prior_evidence(
    session, monkeypatch
):
    s = session
    await configure(s)
    s.policy_model["alias"] = "judge"

    async def judge(self, messages, trace):
        data = json.loads(messages[1]["content"])
        trace["request"] = copy.deepcopy(messages)
        return dict(passed=True, reason="Matches rubric", evidence=data["text"])

    monkeypatch.setattr(FakeRuntime, "judge", judge)
    definition = evaluation.save_definition(
        s.project,
        dict(
            name="Voice",
            kind="llm",
            spec="Coherent",
            prompt=evaluation.DEFAULT_PROMPT,
            model="judge",
            threshold=0.8,
        ),
    )
    await s.execute(
        "evaluation.collection.save",
        {"name": "Voice set", "judges": [definition["id"]]},
        "dataset",
    )
    await s.execute(
        "simulator.run", {"action": "loom", "count": 2, "eval": "Voice set"}, "initial"
    )
    await s.job
    group = s.project.data["evaluation_sets"][-1]
    assert len(group["items"]) == 2
    before_items = copy.deepcopy(group["items"])
    before_evals = copy.deepcopy(s.project.data["evaluations"])
    run = s.project.data["simulation_runs"][0]
    await s.execute(
        "simulator.run",
        {
            "action": "continue",
            "run": run["id"],
            "conversation": 1,
            "visitor": "My chosen question",
            "eval": "Voice set",
        },
        "advance",
    )
    await s.job
    assert len(s.project.data["simulation_runs"]) == 1
    assert len(group["items"]) == 3
    assert group["items"][:2] == before_items
    assert s.project.data["evaluations"][:2] == before_evals
    new = group["items"][-1]
    assert new["target"] == before_items[1]["target"]
    assert new["snapshot_hash"] != before_items[1]["snapshot_hash"]
    assert "My chosen question" in new["text"]
    assert len(new["source"]["conversation"]["turns"]) == 4
    assert new["source"]["revision"] == 1
    assert not new["training"]
    assert not s.busy
    assert not [data for kind, data, _ in s.events if kind == "error"]
