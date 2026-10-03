"""The action contract exercised through the backend command transport."""

import asyncio
from copy import deepcopy

import pytest
from test_service import FakeRuntime
from test_service import session as session

from character_lab.domain import Project
from character_lab.evaluation import capture
from character_lab.evaluation_sets import add_capture


async def execute(session, **args):
    await session.execute("continue", args, "operation")
    await session.job
    assert not [event for event in session.events if event[0] == "error"]


@pytest.mark.asyncio
async def test_continue_same_identity_old_kept_version_stays_pinned(session):
    s = session
    first = s.project.add("A previous generated document.")
    s.project.keep(first["id"])
    frozen = deepcopy(first)
    await execute(s, action="continue", node=first["id"])
    latest = s.current()
    assert latest["document_id"] == first["document_id"]
    assert latest["revision_of"] == first["id"]
    assert latest["text"] == first["text"] + " A path remembers."
    assert not latest["kept"]
    assert s.project.node(first["id"]) == frozen
    state = next(e[1] for e in reversed(s.events) if e[0] == "state")
    assert state["document_heads"][first["document_id"]] == latest["id"]


@pytest.mark.asyncio
async def test_advancement_does_not_transfer_judgment_or_training_marks(session):
    s = session
    original = s.project.add("Judged content.")
    group = {"items": []}
    item = add_capture(group, capture(s.project, {"node": original["id"]}))
    item.update(training=True, judgments=["previous-judge-record"])
    frozen = deepcopy(item)
    await execute(s, action="continue", node=original["id"])
    new_item = add_capture(group, capture(s.project, {"node": s.current()["id"]}))
    assert item == frozen
    assert new_item["id"] != item["id"]
    assert new_item["training"] is False and new_item["judgments"] == []
    assert new_item["source"]["revision_of"] == original["id"]


@pytest.mark.asyncio
async def test_loom_two_document_set_creates_two_alternative_sets(session):
    s = session
    originals = [s.project.add(text) for text in ["First.", "Second."]]
    await execute(s, action="loom", nodes=[n["id"] for n in originals], count=2)
    sets = s.project.data["document_sets"]
    assert len(sets) == 2
    for group in sets:
        members = [s.project.node(key) for key in group["members"]]
        assert [node["prompt"] for node in members] == ["First.", "Second."]
        assert [node["text"] for node in members] == [
            "First. A path remembers.",
            "Second. A path remembers.",
        ]
        assert [node["parent"] for node in members] == [n["id"] for n in originals]
    assert (
        len(
            {
                s.project.node(key)["document_id"]
                for group in sets
                for key in group["members"]
            }
        )
        == 4
    )


@pytest.mark.asyncio
async def test_policy_loops_choose_complete_document_set(session):
    s = session
    originals = [s.project.add(text) for text in ["First.", "Second."]]
    await execute(
        s,
        action="loom",
        nodes=[n["id"] for n in originals],
        count=2,
        loops=2,
        selection=True,
    )
    groups = s.project.data["document_sets"]
    assert len(groups) == 4
    assert all(len(group["members"]) == 2 for group in groups)
    policy = s.project.data["policy_runs"][-1]
    assert policy["status"] == "complete"
    chosen = policy["steps"][0]["decision"]["selected"]
    assert chosen in {group["id"] for group in groups[:2]}
    assert all(group["parent"] == chosen for group in groups[2:])
    selected_group = next(group for group in groups if group["id"] == chosen)
    for group in groups[2:]:
        assert [
            s.project.node(key)["parent"] for key in group["members"]
        ] == selected_group["members"]
    assert len(policy["steps"][0]["decision"]["reviews"]) == 2
    # Every selection candidate contains the entire two-document alternative.
    for review in policy["steps"][0]["decision"]["reviews"]:
        assert "Document 1" in review["evidence"] and "Document 2" in review["evidence"]


@pytest.mark.asyncio
async def test_model_override_is_recorded_and_does_not_replace_defaults(session):
    s = session
    original_runtime = s.runtime
    model = {**s.runtime.model, "alias": "other-base", "name": "Other base"}
    s.project.data["models"].append(model)
    source = s.project.add("First.")
    await execute(s, action="loom", node=source["id"], model="other-base")
    generated = s.current()
    assert generated["trace"]["model"]["alias"] == "other-base"
    assert s.runtime is original_runtime
    assert s.project.data["model_alias"] == original_runtime.model["alias"]
    await execute(s, action="continue", node=generated["id"])
    assert s.current()["trace"]["model"]["alias"] == original_runtime.model["alias"]


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "extra",
    [
        {"offset": -1},
        {"offsets": {}},
        {"node": "missing"},
        {"set": "missing"},
        {"count": 0},
        {"count": True},
        {"model": "unknown"},
        {"visitor": "hello"},
        {"loops": 0},
        {"text": "draft"},
    ],
)
async def test_invalid_library_action_cannot_add_source(session, extra):
    s = session
    before = deepcopy(s.project.data)
    with pytest.raises(ValueError):
        await s.execute(
            "continue", {"action": "loom", "refs": ["meditations:1.1"], **extra}, "bad"
        )
    assert s.project.data == before
    assert not s.busy


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "target", [{"nodes": []}, {"nodes": None}, {"node": None}, {"set": ""}]
)
async def test_invalid_explicit_target_never_falls_back_to_current(session, target):
    s = session
    s.project.add("The preview is not an implicit replacement selection.")
    before = deepcopy(s.project.data)
    with pytest.raises(ValueError):
        await s.execute("continue", {"action": "loom", **target}, "bad")
    assert s.project.data == before and not s.busy


@pytest.mark.asyncio
async def test_cancellation_keeps_partial_revision_and_untouched_history(session):
    class Slow(FakeRuntime):
        async def stream(self, prompt, settings, trace):
            trace.update(request={"prompt": prompt}, model=self.model, events=[])
            yield " Partial."
            await asyncio.sleep(30)

    s = session
    s.runtime = Slow(s.project.folder, s.runtime.model)
    original = s.project.add("First.")
    frozen = deepcopy(original)
    await s.execute("continue", {"action": "continue", "node": original["id"]}, "run")
    async with asyncio.timeout(2):
        while not any(event[0] == "token" for event in s.events):
            await asyncio.sleep(0.001)
    await s.execute("cancel", {}, "stop")
    assert not s.busy
    reopened = Project(s.project.folder)
    latest = reopened.node(reopened.data["document_heads"][original["document_id"]])
    assert latest["status"] == "stopped"
    assert latest["text"] == "First. Partial."
    assert reopened.node(original["id"]) == frozen


@pytest.mark.asyncio
async def test_cancel_before_all_workers_start_preserves_complete_set_shape(session):
    class Slow(FakeRuntime):
        async def stream(self, prompt, settings, trace):
            trace.update(request={"prompt": prompt}, model=self.model, events=[])
            yield " Partial."
            await asyncio.sleep(30)

    s = session
    s.runtime = Slow(s.project.folder, s.runtime.model)
    original = [s.project.add(f"Document {i}.") for i in range(6)]
    await s.execute(
        "continue",
        {"action": "loom", "nodes": [n["id"] for n in original], "count": 2},
        "run",
    )
    async with asyncio.timeout(2):
        while not any(event[0] == "token" for event in s.events):
            await asyncio.sleep(0.001)
    await s.execute("cancel", {}, "stop")
    reopened = Project(s.project.folder)
    groups = reopened.data["document_sets"]
    assert len(groups) == 2
    assert all(len(group["members"]) == 6 for group in groups)
    for group in groups:
        nodes = [reopened.node(key) for key in group["members"]]
        assert all(node["status"] == "stopped" for node in nodes)
        assert [node["parent"] for node in nodes] == [node["id"] for node in original]
        assert all(
            node["text"].startswith(parent["text"])
            for node, parent in zip(nodes, original)
        )


@pytest.mark.asyncio
async def test_loops_extend_each_alternative_without_selector_or_extra_fanout(session):
    s = session
    s.policy_model = {"name": "Unavailable", "path": "/missing-selector"}
    node = s.project.add("Seed.")
    await execute(s, action="loom", node=node["id"], count=2, loops=3)
    assert len(s.project.data["nodes"]) == 7
    heads = [s.project.node(key) for key in s.project.data["document_heads"].values()]
    assert (
        len([n for n in heads if n["text"] == "Seed." + " A path remembers." * 3]) == 2
    )
    assert not s.project.data.get("policy_runs")


@pytest.mark.asyncio
async def test_single_loom_loops_needs_no_selector(session):
    s = session
    s.policy_model = {"name": "Unavailable", "path": "/missing-selector"}
    node = s.project.add("Seed.")
    await execute(s, action="loom", node=node["id"], count=1, loops=3)
    assert s.current()["text"] == "Seed." + " A path remembers." * 3
    assert s.current()["document_id"] == node["document_id"]


@pytest.mark.asyncio
async def test_continue_loops_and_single_loom_ignore_selector(session):
    s = session
    s.policy_model = {"name": "missing", "path": "/missing"}
    node = s.project.add("Seed.")
    s.project.data["selection_enabled"] = True
    await execute(s, action="continue", node=node["id"], loops=2)
    assert s.current()["text"] == "Seed." + " A path remembers." * 2
