"""Dataset semantics and recovery with fake judges, no model or provider spend."""

import asyncio
import copy
import json

import pytest

from character_lab import evaluation
from character_lab.domain import Project
from character_lab.runtime import DEFAULT_MODEL
from character_lab.service import Session
from character_lab.workspaces import Workspaces


class Judge:
    def __init__(self, folder, model):
        self.model = model
        self.closed = False

    def close(self):
        self.closed = True

    async def judge(self, messages, trace):
        text = json.loads(messages[1]["content"])["text"]
        trace["request"] = copy.deepcopy(messages)
        return dict(
            passed="bad" not in text, reason="Applied the rubric", evidence=text
        )


@pytest.fixture
async def lab(tmp_path):
    path = tmp_path / "judge.gguf"
    path.touch()
    events = []

    async def emit(kind, data, request_id=None):
        events.append((kind, copy.deepcopy(data), request_id))

    s = Session(
        tmp_path / "workspace",
        emit,
        models=[DEFAULT_MODEL.copy()],
        policy_model=dict(name="Judge", alias="judge", path=str(path)),
        workspaces=Workspaces(tmp_path / "home"),
        runtime_factory=Judge,
    )
    s.events = events
    yield s
    await s.close()


def define(s, **changes):
    return evaluation.save_definition(
        s.project,
        dict(
            name="Coherence",
            kind="llm",
            spec="Must be coherent",
            prompt=evaluation.DEFAULT_PROMPT,
            model="judge",
            threshold=0.8,
        )
        | changes,
    )


async def run(s, definition, targets, **kwargs):
    await s.execute(
        "evaluation.run",
        dict(definition=definition["id"], targets=targets, **kwargs),
        "eval",
    )
    job = s.job
    await job
    return s.project.data["evaluations"]


@pytest.mark.asyncio
async def test_frozen_input_revision_training_and_export(lab):
    s = lab
    a = s.project.add("A path continues.")
    b = s.project.add("bad repetition")
    definition = define(s)
    records = await run(
        s, definition, [{"node": a["id"]}, {"node": b["id"]}], train_on_pass=True
    )
    assert [r["training"] for r in records] == [True, False]
    assert [r["passed"] for r in records] == [True, False]
    a["text"] = "changed source"
    define(s, **(definition | dict(spec="New criteria")))
    assert records[0]["text"] == "A path continues."
    assert records[0]["source"]["text"] == "A path continues."
    assert records[0]["definition"]["revision"] == 1
    assert evaluation.definitions(s.project)[0]["revision"] == 2
    await s.execute(
        "evaluation.annotate",
        dict(ids=[records[0]["id"]], note="Keep this voice"),
        "note",
    )
    assert records[0]["result"]["passed"] is True
    await s.execute("evaluation.export", {}, "export")
    path = next((s.project.folder / "datasets").glob("*.jsonl"))
    exported = json.loads(path.read_text())
    assert exported["note"] == "Keep this voice"
    assert exported["text"] == "A path continues."
    assert len(exported["metadata_history"]) == 2
    await s.execute(
        "evaluation.annotate", dict(ids=[records[0]["id"]], training=False), "unmark"
    )
    assert json.loads(path.read_text())["training"] is True


@pytest.mark.asyncio
async def test_default_never_marks_training_and_rerun_keeps_snapshot(lab):
    s = lab
    doc = s.project.add("A path.")
    definition = define(s)
    records = await run(s, definition, [{"node": doc["id"]}])
    old = copy.deepcopy(records[0])
    s.project.delete_nodes([doc["id"]], [doc["id"]])
    await run(s, definition, [{"evaluation": old["id"]}])
    assert len(records) == 2
    assert records[0] == old
    assert records[1]["text"] == old["text"]
    assert not records[1]["training"]


@pytest.mark.asyncio
async def test_conversation_capture_includes_all_turns_and_provenance(lab):
    s = lab
    definition = define(s)
    s.project.data["simulation_runs"] = [
        dict(
            id="run",
            status="complete",
            config={
                "temperature": 1,
                "character_alias": "base",
                "visitor_alias": "base",
                "conversations": 2,
            },
            anthology=["source"],
            conversations=[
                dict(
                    index=0,
                    status="complete",
                    turns=[
                        dict(role="visitor", text="Hello"),
                        dict(
                            role="character",
                            text="A path.",
                            model={"alias": "base", "name": "Base"},
                        ),
                    ],
                ),
                dict(
                    index=1,
                    status="complete",
                    turns=[dict(role="visitor", text="Other")],
                ),
            ],
        )
    ]
    records = await run(s, definition, [{"run": "run", "conversation": 0}])
    assert records[0]["text"] == "visitor:\nHello\n\ncharacter:\nA path."
    assert "conversations" not in records[0]["source"]
    assert records[0]["source"]["anthology"] == ["source"]
    assert records[0]["source"]["conversation"]["turns"][1]["model"]["alias"] == "base"


@pytest.mark.asyncio
async def test_invalid_evidence_fails_without_training_and_keeps_raw_result(lab):
    class Bad(Judge):
        async def judge(self, messages, trace):
            return dict(passed=True, reason="invented", evidence="not in source")

    lab.runtime_factory = Bad
    doc = lab.project.add("A path.")
    records = await run(lab, define(lab), [{"node": doc["id"]}], train_on_pass=True)
    assert records[0]["status"] == "failed"
    assert records[0]["raw_result"]["passed"] is True
    assert not records[0]["training"]
    with pytest.raises(ValueError, match="Finish evaluation"):
        await lab.execute(
            "evaluation.annotate", dict(ids=[records[0]["id"]], training=True), "mark"
        )


@pytest.mark.asyncio
async def test_cancel_preserves_finished_results_and_stops_remaining(lab):
    entered = asyncio.Event()

    class Slow(Judge):
        async def judge(self, messages, trace):
            if "second" in messages[1]["content"]:
                entered.set()
                await asyncio.Event().wait()
            return await super().judge(messages, trace)

    lab.runtime_factory = Slow
    nodes = [lab.project.add(text) for text in ("first", "second", "third")]
    await lab.execute(
        "evaluation.run",
        dict(definition=define(lab)["id"], targets=[{"node": n["id"]} for n in nodes]),
        "run",
    )
    await entered.wait()
    await lab.execute("cancel", {}, "stop")
    records = lab.project.data["evaluations"]
    assert [r["status"] for r in records] == ["complete", "stopped", "stopped"]
    assert not lab.busy


@pytest.mark.asyncio
async def test_jev_uses_entire_trace_and_errors_do_not_pass(lab, monkeypatch):
    requests = []

    async def classify(request, record):
        requests.append(request)
        record.update(status="complete", scores={"passes": 0.81}, request=request)

    monkeypatch.setattr(evaluation, "classify", classify)
    doc = lab.project.add("Full document text")
    definition = define(lab, kind="jev", model="jev", threshold=0.8)
    records = await run(lab, definition, [{"node": doc["id"]}], train_on_pass=True)
    assert requests[0]["state"]["text"] == doc["text"]
    assert records[0]["training"]

    async def unavailable(request, record):
        record.update(status="unavailable", error="No key")

    monkeypatch.setattr(evaluation, "classify", unavailable)
    await run(lab, definition, [{"node": doc["id"]}], train_on_pass=True)
    assert records[1]["status"] == "failed" and not records[1]["training"]


def test_recovery_marks_pending_evaluations_interrupted(tmp_path):
    p = Project(tmp_path)
    p.data["evaluations"] = [dict(status=s) for s in ("queued", "running", "complete")]
    p.save()
    assert [r["status"] for r in Project(tmp_path).data["evaluations"]] == [
        "interrupted",
        "interrupted",
        "complete",
    ]


@pytest.mark.asyncio
async def test_immediate_cancel_and_disconnect_preserve_queue_state(lab):
    doc = lab.project.add("A path.")
    definition = define(lab)
    for action in ("cancel", "close"):
        await lab.execute(
            "evaluation.run",
            dict(definition=definition["id"], targets=[{"node": doc["id"]}]),
            "run",
        )
        if action == "cancel":
            await lab.execute("cancel", {}, "stop")
        else:
            await lab.close()
        assert lab.project.data["evaluations"][-1]["status"] == "stopped"
        assert not lab.busy


@pytest.mark.asyncio
async def test_edited_document_freezes_ancestor_generation_evidence(lab):
    parent = lab.project.add(
        "Original", trace={"model": {"name": "Base"}, "request": {"prompt": "Seed"}}
    )
    edited = lab.project.add("Edited", parent=parent["id"], kind="edited")
    records = await run(lab, define(lab), [{"node": edited["id"]}])
    ancestor = records[0]["source"]["ancestors"][0]
    assert ancestor["trace"]["model"]["name"] == "Base"
    assert ancestor["trace"]["request"]["prompt"] == "Seed"


async def collection(s, definition, name="Voice"):
    await s.execute(
        "evaluation.collection.save",
        {"name": name, "judges": [definition["id"]]},
        "new-set",
    )
    return s.project.data["evaluation_sets"][-1]


@pytest.mark.asyncio
async def test_collections_add_run_attach_and_preserve_original_evidence(lab):
    from character_lab import evaluation_sets as sets

    s = lab
    definition = define(s)
    group = await collection(s, definition)
    node = s.project.add("A coherent path.")
    await s.execute(
        "evaluation.collection.add",
        {"collection": group["id"], "targets": [{"node": node["id"]}]},
        "add",
    )
    assert not s.busy and not s.project.data.get("evaluations")
    item = group["items"][0]
    assert sets.item_summary(s.project, group, item)["status"] == "unjudged"
    await s.execute(
        "evaluation.collection.run",
        {"collection": "Voice", "items": [item["id"]], "train_on_pass": True},
        "judge",
    )
    await s.job
    assert (
        item["training"] and sets.item_summary(s.project, group, item)["passed"] is True
    )
    original = copy.deepcopy(s.project.data["evaluations"])
    second = await collection(s, definition, "Comparison")
    await s.execute(
        "evaluation.collection.add",
        {"collection": second["id"], "targets": [{"evaluation": original[0]["id"]}]},
        "attach",
    )
    assert not s.busy
    assert s.project.data["evaluations"] == original
    assert second["items"][0]["judgments"] == [original[0]["id"]]
    await s.execute(
        "evaluation.item.open",
        {"collection": second["id"], "id": second["items"][0]["id"]},
        "open",
    )
    assert s.events[-1][1]["evidence"] == []
    assert len(s.events[-1][1]["judgments"]) == 1
    define(s, id=definition["id"], spec="New criteria")
    assert (
        sets.item_summary(s.project, group, item)["status"] == "complete"
    )  # policy owns its copied behavior
    assert s.project.data["evaluations"] == original


@pytest.mark.asyncio
async def test_collection_migration_is_additive_and_idempotent(lab):
    from character_lab import evaluation_sets as sets

    s = lab
    definition = define(s)
    node = s.project.add("Old document")
    await run(s, definition, [{"node": node["id"]}])
    original = copy.deepcopy(s.project.data["evaluations"])
    del s.project.data["evaluation_sets"]
    sets.migrate(s.project)
    groups = copy.deepcopy(sets.collections(s.project))
    sets.migrate(s.project)
    assert sets.collections(s.project) == groups
    assert groups[0]["items"][0]["judgments"] == [original[0]["id"]]
    assert s.project.data["evaluations"] == original


@pytest.mark.asyncio
async def test_attach_monitoring_and_selection_is_scoped_not_whole_item_pass(lab):
    from character_lab import evaluation_sets as sets

    s = lab
    group = await collection(s, define(s))
    node = s.project.add("A path repeats")
    node["monitor_checks"] = [
        {
            "status": "complete",
            "characters": 4,
            "end_of_turn": False,
            "scores": {"looping": 0.9},
        }
    ]
    s.project.data["policy_runs"] = [
        {
            "id": "selection",
            "spec": "Develops the theme",
            "prompt": "judge",
            "policy_model": {"alias": "judge"},
            "steps": [
                {
                    "loop": 1,
                    "status": "complete",
                    "candidates": [node["id"]],
                    "messages": ["exact candidate context"],
                    "decision": {"selected": node["id"]},
                }
            ],
        }
    ]
    args = {
        "collection": group["id"],
        "targets": [{"node": node["id"]}],
        "attach_evidence": True,
    }
    await s.execute("evaluation.collection.add", args, "attach")
    await s.execute("evaluation.collection.add", args, "again")
    item = group["items"][0]
    assert len(group["items"]) == 1 and len(item["evidence"]) == 2
    assert sets.item_summary(s.project, group, item)["passed"] is None
    assert item["evidence"][0]["result"]["characters"] == 4
    assert not s.busy


@pytest.mark.asyncio
async def test_loom_evaluation_chain_and_preflight(lab, monkeypatch):
    from test_service import FakeRuntime

    s = lab
    monkeypatch.setattr(Judge, "stream", FakeRuntime.stream, raising=False)
    monkeypatch.setattr(Judge, "preflight", FakeRuntime.preflight, raising=False)
    definition = define(s)
    group = await collection(s, definition)
    node = s.project.add("Seed")
    before = len(s.project.data["nodes"])
    with pytest.raises(ValueError):
        await s.execute("continue", {"node": node["id"], "eval": "missing"}, "bad")
    assert len(s.project.data["nodes"]) == before
    await s.execute(
        "continue", {"node": node["id"], "count": 2, "eval": "Voice"}, "chain"
    )
    await s.job
    assert len(group["items"]) == 2
    assert all(i["judgments"] for i in group["items"])
    assert all(r["passed"] for r in s.project.data["evaluations"])
    assert not s.busy


@pytest.mark.asyncio
async def test_simulator_chain_freezes_conversations_and_runs_all_judges(
    lab, monkeypatch
):
    from test_service import FakeRuntime

    from character_lab import evaluation_sets as sets

    s = lab
    monkeypatch.setattr(Judge, "stream", FakeRuntime.stream, raising=False)
    monkeypatch.setattr(Judge, "preflight", FakeRuntime.preflight, raising=False)
    first, second = define(s), define(s, name="Another rubric")
    group = await collection(s, first)
    await s.execute(
        "evaluation.collection.save",
        {
            "id": group["id"],
            "name": group["name"],
            "judges": [first["id"], second["id"]],
        },
        "judges",
    )
    node = s.project.add("Anthology.")
    node["kept"] = True
    await s.execute(
        "simulator.configure",
        {
            "documents": [node["id"]],
            "turns": 1,
            "opening_mode": "fixed",
            "opening": "Hello",
        },
        "config",
    )
    await s.execute("simulator.run", {"count": 2, "eval": group["name"]}, "chain")
    await s.job
    assert len(group["items"]) == 2
    assert all(len(item["judgments"]) == 4 for item in group["items"])
    assert all(
        sets.item_summary(s.project, group, item)["passed"] for item in group["items"]
    )
    statuses = [data["busy"] for kind, data, _ in s.events if kind == "state"]
    # Once generation starts, ownership stays held through judging until final snapshot.
    first_busy = statuses.index(True)
    assert all(statuses[first_busy:-1]) and statuses[-1] is False


@pytest.mark.asyncio
async def test_cancel_generation_never_starts_chained_judge(lab, monkeypatch):
    s = lab
    started = asyncio.Event()

    async def stream(self, prompt, settings, trace):
        started.set()
        yield "partial"
        await asyncio.Event().wait()

    monkeypatch.setattr(Judge, "stream", stream, raising=False)
    group = await collection(s, define(s))
    node = s.project.add("Seed")
    await s.execute("continue", {"node": node["id"], "eval": group["name"]}, "chain")
    await started.wait()
    await s.execute("cancel", {}, "cancel")
    assert not group["items"]
    assert not s.project.data.get("evaluations")
    assert not s.busy


async def test_diffusion_evaluation_freezes_endpoint_and_failure_cannot_train(
    lab, monkeypatch
):
    import httpx

    from character_lab import monitor

    s = lab
    doc = s.project.add("A coherent document.")
    definition = define(
        s, kind="diffusion", model=monitor.LOCAL_MODEL, endpoint="http://localhost:8080"
    )
    requests = []

    def handle(request):
        requests.append(request)
        assert "authorization" not in request.headers
        assert str(request.url) == "http://127.0.0.1:8080/v1/systemone"
        if len(requests) > 1:
            return httpx.Response(503)
        return httpx.Response(
            200, json={"model": "openjev-0.1", "answers": {"passes": {"noul": 0.91}}}
        )

    real = httpx.AsyncClient
    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: real(transport=httpx.MockTransport(handle), **kw),
    )
    records = await run(s, definition, [{"node": doc["id"]}], train_on_pass=True)
    assert records[0]["passed"] and records[0]["training"]
    assert records[0]["trace"]["provider"] == "openjev"
    define(s, **(definition | {"endpoint": "http://localhost:8081"}))
    assert records[0]["definition"]["endpoint"] == "http://127.0.0.1:8080"
    # Restore the endpoint; a server failure must not become a pass/training signal.
    definition = define(s, **definition)
    await run(s, definition, [{"node": doc["id"]}], train_on_pass=True)
    assert records[-1]["status"] == "failed" and not records[-1]["training"]


@pytest.mark.asyncio
async def test_collection_remove_preserves_frozen_evidence_and_is_atomic(lab):
    from character_lab import evaluation_sets

    s = lab
    node = s.project.add("Exact frozen input")
    group = dict(id="collection", name="Examples", judges=[], items=[])
    s.project.data["evaluation_sets"] = [group]
    item = evaluation_sets.add_capture(
        group, evaluation.capture(s.project, {"node": node["id"]})
    )
    item.update(training=True, judgments=["saved-judgment"])
    before = copy.deepcopy(group)
    with pytest.raises(ValueError):
        await s.execute(
            "evaluation.collection.remove",
            {"collection": "collection", "ids": [item["id"], "missing"]},
            "bad",
        )
    assert group == before
    await s.execute(
        "evaluation.collection.remove",
        {"collection": "collection", "ids": [item["id"]]},
        "remove",
    )
    assert group["items"] == []
    assert group["removed_items"][0]["item"] == item
    assert s.project.node(node["id"])["text"] == "Exact frozen input"


def test_group_selection_evidence_attaches_by_set_not_leaf(tmp_path):
    from character_lab import evaluation_sets

    p = Project(tmp_path)
    p.data["policy_runs"] = [
        dict(
            id="policy",
            spec="coherence",
            prompt="judge",
            policy_model={},
            steps=[dict(status="complete", loop=1, candidates=["0", "1"])],
        )
    ]
    item = dict(
        evidence=[],
        kind="conversation",
        target={"run": "r", "conversation": 3},
        source=dict(
            policy_run="policy",
            loop=1,
            alternative_group="g",
            alternative_index=1,
            conversation={"turns": []},
        ),
    )
    evaluation_sets.attach_policy_evidence(p, item)
    assert len(item["evidence"]) == 1
    assert item["evidence"][0]["scope"] == "candidate set"


@pytest.mark.asyncio
async def test_remove_collection_preserves_evidence_and_stays_removed(lab):
    from character_lab import evaluation_sets

    s = lab
    node = s.project.add("Exact source material")
    group = await collection(s, define(s))
    item = evaluation_sets.add_capture(
        group, evaluation.capture(s.project, {"node": node["id"]})
    )
    await s.execute(
        "evaluation.collection.run",
        {"collection": group["id"], "items": [item["id"]]},
        "run",
    )
    await s.job
    before = copy.deepcopy(s.project.data)
    for key in (None, "missing"):
        with pytest.raises(ValueError):
            await s.execute(
                "evaluation.collection.delete", {"collection": key}, "invalid"
            )
        assert s.project.data == before
    second = await collection(s, define(s, name="Other"), "Other")
    await s.execute(
        "evaluation.collection.active", {"collection": group["id"]}, "active"
    )
    await s.execute(
        "evaluation.collection.delete", {"collection": group["id"]}, "delete"
    )
    assert s.project.data["active_evaluation"] == second["id"]
    assert (
        s.project.data["removed_evaluation_sets"][0]["collection"]
        == before["evaluation_sets"][0]
    )
    await s.execute(
        "evaluation.collection.delete", {"collection": second["id"]}, "delete-last"
    )
    assert s.project.data["active_evaluation"] == ""
    for key in ("nodes", "evaluations", "evaluation_runs"):
        assert s.project.data[key] == before[key]
    policies = copy.deepcopy(s.project.data["evaluation_policies"])
    reloaded = Project(s.project.folder)
    evaluation_sets.migrate(reloaded)
    assert reloaded.data["evaluation_sets"] == []
    assert reloaded.data["evaluation_policies"] == policies
    assert len(reloaded.data["removed_evaluation_sets"]) == 2
