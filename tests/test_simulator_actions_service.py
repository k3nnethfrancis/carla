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
async def test_simulator_loop_reports_underlying_context_error(session, monkeypatch):
    await configure(session)
    error = "Input needs 8213 tokens plus 641 requested output tokens; context is 8192. Select less text. Nothing was truncated."

    async def overflow(self, prompt, settings, trace):
        trace.update(prompt_tokens=8213, context_capacity=8192)
        raise ValueError(error)
        yield  # This replacement has the runtime's async-generator interface.

    monkeypatch.setattr(FakeRuntime, "stream", overflow)
    await session.execute(
        "simulator.run", dict(action="loom", count=2, turns=3, loops=2), "overflow"
    )
    await session.job
    runs = session.project.data["simulation_runs"]
    assert len(runs) == 1 and runs[0]["status"] == "failed"
    assert runs[0]["error"] == error
    assert len(runs[0]["conversations"]) == 2
    assert any(
        kind == "error" and error in data["message"] for kind, data, _ in session.events
    )
    assert not session.busy


@pytest.mark.asyncio
async def test_fresh_context_preflight_leaves_no_failed_runs(session, monkeypatch):
    await configure(session)
    calls = []

    async def reject(self, prompt, settings):
        calls.append((prompt, settings))
        raise ValueError("Input needs 8213 tokens; context is 8192")

    monkeypatch.setattr(FakeRuntime, "preflight", reject)
    await session.execute(
        "simulator.run", dict(action="loom", count=2, turns=3, loops=2), "check"
    )
    await session.job
    assert len(calls) == 1 and "A synthetic anthology." in calls[0][0]
    assert not session.project.data.get("simulation_runs")
    assert any(
        kind == "error" and "8213" in data["message"]
        for kind, data, _ in session.events
    )
    assert not session.busy


@pytest.mark.asyncio
async def test_simulator_context_edits_selected_model_without_retargeting_generator(
    session,
):
    await configure(session)
    models = session.project.data["models"]
    models.append({**models[0], "alias": "second", "context": 8192})
    original = session.runtime.model["alias"]
    await session.execute(
        "simulator.configure",
        dict(character_alias="second", visitor_alias="second", character_context=32768),
        "context",
    )
    assert models[-1]["context"] == 32768
    assert session.runtime.model["alias"] == original
    assert session.state()["model_contexts"]["second"]["configured"] == 32768
    await session.execute("simulator.configure", dict(visitor_context=0), "native")
    assert models[-1]["context"] == 0
    with pytest.raises(ValueError, match="same model"):
        await session.execute(
            "simulator.configure",
            dict(character_context=8192, visitor_context=16384),
            "conflict",
        )
    assert models[-1]["context"] == 0


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
            "selection": True,
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
    assert group["items"] == []
    before_items = copy.deepcopy(s.project.data["evaluations"])
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
    assert group["items"] == []
    assert len(s.project.data["evaluations"]) == 3
    assert s.project.data["evaluations"][:2] == before_evals
    new = s.project.data["evaluations"][-1]
    assert new["target"] == before_items[1]["target"]
    assert new["content_hash"] != before_items[1]["content_hash"]
    assert "My chosen question" in new["text"]
    assert len(new["source"]["conversation"]["turns"]) == 4
    assert new["source"]["revision"] == 1
    assert not new["training"]
    assert not s.busy
    assert not [data for kind, data, _ in s.events if kind == "error"]


@pytest.mark.asyncio
async def test_scope_loops_advance_same_leaves_and_nested_loom_preserves_shape(session):
    from character_lab import action_scope

    s = session
    await configure(s)
    await s.execute("simulator.run", {"action": "loom", "count": 2}, "fresh")
    await s.job
    original = s.project.data["simulation_runs"][0]
    scope = dict(
        kind="set",
        id=original["id"],
        children=[
            dict(kind="conversation", run=original["id"], conversation=i)
            for i in range(2)
        ],
    )
    await s.execute(
        "simulator.run", dict(action="loom", scope=scope, loops=2), "advance"
    )
    await s.job
    assert len(s.project.data["simulation_runs"]) == 1
    assert [len(c["turns"]) for c in original["conversations"]] == [6, 6]
    assert original["revision"] == 2
    assert not s.project.data.get("policy_runs")

    await s.execute(
        "simulator.run", dict(action="loom", scope=scope, count=2, loops=2), "fork"
    )
    await s.job
    runs = s.project.data["simulation_runs"]
    assert len(runs) == 3
    assert all(len(c["turns"]) == 10 for r in runs[1:] for c in r["conversations"])
    nested = dict(
        kind="set",
        id=runs[1]["alternative_group"],
        children=[r["alternative_scope"] for r in runs[1:]],
    )
    before = copy.deepcopy(runs)
    await s.execute(
        "simulator.run", dict(action="loom", scope=nested, count=2), "nested"
    )
    await s.job
    assert runs[:3] == before
    assert len(runs) == 7
    for r in runs[3:]:
        assert r["source_scope"] == nested
        tree = r["alternative_scope"]
        assert len(tree["children"]) == 2
        assert all(len(child["children"]) == 2 for child in tree["children"])
        assert len(list(action_scope.leaves(tree))) == 4
    await s.execute(
        "simulator.run",
        dict(action="loom", scope=runs[3]["alternative_scope"]),
        "nested-continue",
    )
    await s.job
    assert len(runs) == 7
    assert all(len(c["turns"]) == 14 for r in runs[3:5] for c in r["conversations"])
    before = copy.deepcopy(runs)
    await s.execute("simulator.fork", dict(scope=nested), "copy-nested")
    assert runs[:7] == before
    assert len(runs) == 9
    assert all(r["status"] == "draft" for r in runs[7:])
    assert all(
        len(list(action_scope.leaves(r["alternative_scope"]))) == 4 for r in runs[7:]
    )
    assert not [data for kind, data, _ in s.events if kind == "error"]


@pytest.mark.asyncio
async def test_documents_override_is_ephemeral_and_duplicate_scope_rejected(session):
    s = session
    await configure(s)
    saved = copy.deepcopy(s.project.data["simulator_config"])
    doc = s.project.add("Another precise seed", kind="source")
    doc["kept"] = True
    await s.execute(
        "simulator.run", dict(action="loom", documents=[doc["id"]]), "anthology"
    )
    await s.job
    assert s.project.data["simulator_config"] == saved
    run = s.project.data["simulation_runs"][0]
    assert run["documents"][0]["text"] == doc["text"]
    leaf = dict(kind="conversation", run=run["id"], conversation=0)
    with pytest.raises(ValueError, match="unique"):
        await s.execute(
            "simulator.run",
            dict(action="loom", scope=dict(kind="set", children=[leaf, leaf])),
            "duplicate",
        )
    assert len(s.project.data["simulation_runs"]) == 1


@pytest.mark.asyncio
async def test_leaf_scope_forks_beneath_exact_conversation_and_group_opens_while_running(
    session,
):
    s = session
    await configure(s)
    await s.execute("simulator.run", dict(action="loom", count=4), "first")
    await s.job
    original = s.project.data["simulation_runs"][0]
    leaf = dict(kind="conversation", run=original["id"], conversation=2)
    await s.execute(
        "simulator.run", dict(action="loom", scope=leaf, count=2), "leaf-fork"
    )
    await s.job
    forks = s.project.data["simulation_runs"][1:]
    assert all(r["parent"] == dict(run=original["id"], conversation=2) for r in forks)
    scope = dict(
        kind="set",
        id=forks[0]["alternative_group"],
        children=[r["alternative_scope"] for r in forks],
    )
    forks[0]["status"] = "running"
    await s.execute("simulator.open", dict(scope=scope), "inspect")
    opened = [
        data
        for kind, data, request in s.events
        if kind == "simulation" and request == "inspect"
    ]
    assert len(opened) == 2
    assert all(data["grid_group"] == scope["id"] for data in opened)
    forks[0]["status"] = "complete"
