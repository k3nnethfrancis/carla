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
