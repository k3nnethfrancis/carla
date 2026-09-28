"""Behavioral contract tests run without a terminal or a downloaded model."""

import asyncio
import json

import pytest

from character_lab.domain import Project
from character_lab.runtime import DEFAULT_MODEL
from character_lab.service import Session
from character_lab.workspaces import Workspaces, acquire


class FakeRuntime:
    def __init__(self, folder, model):
        self.model = model
        self.process = object()
        self.closed = False

    async def stream(self, prompt, settings, trace):
        trace.update(request={"prompt": prompt}, model=self.model, events=[])
        for chunk in [" A path", " remembers."]:
            trace["events"].append({"content": chunk})
            yield chunk
            await asyncio.sleep(0)

    async def judge(self, messages, trace):
        trace["messages"] = messages
        data = json.loads(messages[1]["content"])
        candidates = data["candidates"]
        return {
            "reviews": [
                dict(
                    node=c["node"],
                    decision="explore",
                    reason="Coherent development",
                    evidence=c["continuation"],
                )
                for c in candidates
            ],
            "selected": candidates[0]["node"],
            "reason": "Develops the path",
        }

    def close(self):
        self.closed = True


@pytest.fixture
async def session(tmp_path):
    events = []

    async def emit(kind, data, request_id=None):
        events.append((kind, json.loads(json.dumps(data)), request_id))

    model = tmp_path / "policy.gguf"
    model.touch()
    s = Session(
        tmp_path / "first",
        emit,
        workspaces=Workspaces(tmp_path / "workspaces"),
        runtime_factory=FakeRuntime,
        policy_model=dict(name="Test policy", path=str(model)),
    )
    s.events = events
    yield s
    await s.close()


@pytest.mark.asyncio
async def test_exact_prefix_branches_and_ordered_raw_tokens(session):
    s = session
    root = s.project.add("Source 🙂 with text.", kind="source")
    await s.execute("continue", {"offset": 8, "branch": True}, "generate-1")
    await s.job
    nodes = s.project.data["nodes"][1:]
    assert len(nodes) == 3
    assert all(n["parent"] == root["id"] and n["prompt"] == "Source 🙂" for n in nodes)
    assert all(n["text"] == "Source 🙂 A path remembers." for n in nodes)
    # Independent branches interleave; each branch's own tokens stay ordered.
    for node in nodes:
        assert [
            e[1]["text"]
            for e in s.events
            if e[0] == "token" and e[1]["node"] == node["id"]
        ] == [" A path", " remembers."]
    assert all(e[2] == "generate-1" for e in s.events if e[0] == "token")
    assert s.events[-2][0] == "state" and not s.events[-2][1]["busy"]
    assert s.events[-1][1]["stage"] == "complete"
    assert nodes[0]["trace"]["request"]["prompt"] == "Source 🙂"


@pytest.mark.asyncio
async def test_cancel_preserves_partial_and_blocks_switch(session):
    class Slow(FakeRuntime):
        async def stream(self, *args):
            yield " Partial."
            await asyncio.sleep(30)

    s = session
    s.runtime = Slow(s.project.folder, DEFAULT_MODEL)
    s.project.add("Seed", kind="source")
    await s.execute("continue", {}, "run")
    while not any(e[0] == "token" for e in s.events):
        await asyncio.sleep(0.001)
    with pytest.raises(ValueError, match="Stop"):
        await s.execute("workspace.open", {"name": "other"}, "open")
    await s.execute("cancel", {}, "stop")
    saved = Project(s.project.folder)
    assert saved.node(saved.data["current"])["status"] == "stopped"
    assert saved.node(saved.data["current"])["text"] == "Seed Partial."
    assert not s.busy


@pytest.mark.asyncio
async def test_clear_edit_keep_review_snapshot_and_restart(session):
    s = session
    await s.execute("seed.toggle", {"ref": "meditations:1.1"}, "select")
    root = s.current().copy()
    await s.execute(
        "node.edit", {"node": root["id"], "text": "Source\nA path remembers."}, "edit"
    )
    edited = s.current().copy()
    assert s.project.node(root["id"]) == root
    await s.execute(
        "annotate",
        {"start": 7, "end": 24, "verdict": "promising", "note": "Keep the image"},
        "review",
    )
    await s.execute("node.keep", {}, "keep")
    await s.execute("snapshot", {}, "snapshot")
    from pathlib import Path

    manifest = json.loads(
        (Path(s.project.data["snapshots"][-1]) / "manifest.json").read_text()
    )
    assert manifest["annotations"][0]["quote"] == "A path remembers."
    await s.execute("seed.clear", {}, "clear")
    assert s.project.selected == [] and s.project.data["current"] == edited["id"]
    assert s.project.node(edited["id"])["kept"]
    await s.execute("node.open", {"node": edited["id"]}, "open")
    await s.execute("continue", {}, "continue")
    await s.job
    assert "Keep the image" not in s.current()["prompt"]


@pytest.mark.asyncio
async def test_workspaces_model_persistence_and_busy_lock(session):
    s = session
    first = s.project.folder
    other_model = {**DEFAULT_MODEL, "alias": "other", "name": "Other base"}
    s.project.data["models"].append(other_model)
    await s.execute(
        "configure", {"model_alias": "other", "settings": {"n_predict": 42}}, "config"
    )
    s.project.add("First document", kind="source")
    await s.execute("workspace.open", {"name": "second", "create": True}, "switch")
    assert s.project.selected == [] and s.project.data["nodes"] == []
    with acquire(first):
        with pytest.raises(ValueError, match="already open"):
            await s.execute("workspace.open", {"path": str(first)}, "busy")
    assert s.project.folder.name == "second"
    await s.execute("workspace.open", {"path": str(first)}, "return")
    assert s.runtime.model["alias"] == "other"
    assert s.state()["settings"]["n_predict"] == 42
    assert s.current()["text"] == "First document"


@pytest.mark.asyncio
async def test_policy_uses_separate_prompts_and_does_not_keep(session):
    s = session
    s.project.add("Seed", kind="source")
    await s.execute(
        "configure",
        {"policy_spec": "Follow the image", "settings": {"count": 2, "rounds": 2}},
        "config",
    )
    await s.execute("grow", {}, "grow")
    await s.job
    run = s.project.data["policy_runs"][-1]
    assert run["status"] == "complete" and len(run["steps"]) == 2
    assert run["spec"] == "Follow the image"
    assert run["steps"][1]["parent"] == run["steps"][0]["decision"]["selected"]
    assert all(
        "Follow the image" not in n.get("prompt", "") for n in s.project.data["nodes"]
    )
    assert not any(n["kept"] for n in s.project.data["nodes"])
    await s.execute("inspect", {"run": run["id"]}, "inspect")
    assert s.events[-1][0] == "inspection"
    assert (
        s.events[-1][1]["steps"][0]["decision"]["reviews"][0]["evidence"]
        == "A path remembers."
    )


@pytest.mark.asyncio
async def test_immediate_cancel_and_invalid_edit_are_non_destructive(session):
    s = session
    root = s.project.add("Seed", kind="source")
    await s.execute("continue", {}, "run")
    await s.execute("cancel", {}, "cancel")
    assert not s.busy
    assert s.current()["id"] == root["id"]
    with pytest.raises(ValueError, match="changed"):
        await s.execute("node.edit", {"node": "stale", "text": "Overwrite"}, "edit")
    assert s.current()["text"] == "Seed"


@pytest.mark.asyncio
async def test_explicit_membership_and_continuation_targets(session):
    s = session
    first = s.project.add("First source", kind="source")
    other = s.project.add("Other source", kind="source")
    for _ in range(2):
        await s.execute("node.keep", {"node": first["id"], "kept": True}, "keep")
    assert first["kept"] and not other["kept"]
    assert s.project.data["current"] == other["id"]
    await s.execute("node.keep", {"node": first["id"], "kept": False}, "remove")
    assert not first["kept"] and len(s.project.data["nodes"]) == 2
    ref = s.sources[0]["key"] + ":" + s.sources[0]["passages"][0]["id"]
    for _ in range(2):
        await s.execute("seed.add", {"refs": [ref]}, "add")
    assert s.project.selected == [ref]
    await s.execute("seed.remove", {"refs": [ref]}, "remove")
    assert s.project.selected == [] and s.project.node(first["id"])
    await s.execute("continue", {"node": first["id"]}, "continue-node")
    await s.job
    assert s.project.data["nodes"][-1]["parent"] == first["id"]
    await s.execute("continue", {"refs": [ref]}, "continue-source")
    await s.job
    generated = s.project.data["nodes"][-1]
    assert generated["prompt"] == s.sources[0]["passages"][0]["text"]
    assert s.project.selected == []
    assert s.project.node(generated["parent"])["passage_ids"] == [ref]


@pytest.mark.asyncio
async def test_bindings_saved_across_workspaces(session):
    s = session
    bindings = {"models": "ctrl+y", "nav.next": "ctrl+j", "continue": ""}
    await s.execute("bindings.save", {"bindings": bindings}, "bindings")
    assert s.state()["bindings"] == bindings
    await s.execute("workspace.open", {"name": "another", "create": True}, "open")
    assert s.state()["bindings"] == bindings
    assert "show_keys" not in s.state()
    with pytest.raises(ValueError):
        await s.execute("bindings.save", {"bindings": {"models": 2}}, "invalid")
    assert s.read_bindings() == bindings


@pytest.mark.asyncio
async def test_open_seed_set_without_generation_or_duplicates(session):
    s = session
    with pytest.raises(ValueError, match="Space"):
        await s.execute("seed.open", {}, "empty")
    refs = [
        source["key"] + ":" + source["passages"][0]["id"] for source in s.sources[:2]
    ]
    await s.execute("seed.add", {"refs": refs}, "select")
    count = len(s.project.data["nodes"])
    await s.execute("seed.open", {}, "open")
    root = s.current()
    await s.execute("seed.open", {}, "open-again")
    assert s.current()["id"] == root["id"]
    assert len(s.project.data["nodes"]) == count
    assert set(root["passage_ids"]) == set(refs)
    assert s.job is None


@pytest.mark.asyncio
async def test_batch_keep_and_reviewed_subtree_delete(session):
    s = session
    root = s.project.add("Root", kind="source")
    child = s.project.add("Child", parent=root["id"])
    leaf = s.project.add("Leaf", parent=child["id"])
    sibling = s.project.add("Sibling", parent=root["id"])
    await s.execute("node.keep", {"nodes": [child["id"], sibling["id"]]}, "keep")
    assert child["kept"] and sibling["kept"] and not leaf["kept"]
    before = json.loads(json.dumps(s.project.data))
    with pytest.raises(ValueError, match="review"):
        await s.execute(
            "node.delete", {"nodes": [child["id"]], "expected": [child["id"]]}, "stale"
        )
    assert s.project.data == before
    s.project.annotate(leaf["id"], 0, 4, "promising", "note")
    s.project.data["policy_runs"] = [{"id": "run", "selected": leaf["id"]}]
    s.project.data["current"] = leaf["id"]
    await s.execute(
        "node.delete",
        {"nodes": [child["id"]], "expected": [child["id"], leaf["id"]]},
        "delete",
    )
    assert {n["id"] for n in s.project.data["nodes"]} == {root["id"], sibling["id"]}
    assert s.project.data["current"] == sibling["id"]
    assert not s.project.data["annotations"] and not s.project.data["policy_runs"]
    archives = list((s.project.folder / "deleted").glob("*.json"))
    archived = json.loads(archives[0].read_text())
    assert (
        len(archived["nodes"]) == 4
        and archived["annotations"]
        and archived["policy_runs"]
    )


@pytest.mark.asyncio
async def test_gunkel_only_does_not_include_tractatus(session):
    s = session
    gunkel = next(source for source in s.sources if source["key"] == "gunkel")
    tractatus = next(source for source in s.sources if source["key"] == "tractatus")
    grefs = [gunkel["key"] + ":" + p["id"] for p in gunkel["passages"]]
    trefs = [tractatus["key"] + ":" + p["id"] for p in tractatus["passages"]]
    await s.execute("seed.add", {"refs": trefs + grefs}, "mixed")
    await s.execute("seed.open", {}, "open-mixed")
    mixed = s.current()
    assert tractatus["passages"][0]["text"] in mixed["text"]
    await s.execute("seed.remove", {"refs": trefs}, "remove-tractatus")
    await s.execute("seed.open", {}, "open-gunkel")
    assert s.current()["text"] == "\n\n".join(p["text"] for p in gunkel["passages"])
    assert s.current()["passage_ids"] == grefs
    assert s.project.node(mixed["id"])["text"] == mixed["text"]


@pytest.mark.asyncio
async def test_reopening_workspace_clears_selection_not_branches(session):
    s = session
    ref = s.sources[0]["key"] + ":" + s.sources[0]["passages"][0]["id"]
    await s.execute("seed.add", {"refs": [ref]}, "add")
    folder = s.project.folder
    nodes = json.loads(json.dumps(s.project.data["nodes"]))
    await s.execute("workspace.open", {"name": "other", "create": True}, "other")
    await s.execute("workspace.open", {"path": str(folder)}, "back")
    assert s.project.selected == [] and s.project.data["nodes"] == nodes


@pytest.mark.asyncio
async def test_immediate_eos_is_visible_and_trace_is_preserved(session):
    s = session
    root = s.project.add("Finished source.", kind="source")

    async def eos(prompt, settings, trace):
        trace["events"] = [
            {"stop": True, "stop_type": "eos", "tokens_predicted": 1, "content": ""}
        ]
        yield ""

    s.runtime.stream = eos
    await s.execute("continue", {"node": root["id"]}, "empty")
    await s.job
    node = s.current()
    assert node["text"] == node["prompt"] == root["text"]
    assert node["status"] == "empty"
    assert node["trace"]["events"][0]["tokens_predicted"] == 1
    assert "EOS" in s.events[-1][1]["message"]
    # Historical attempts receive the same truthful label without rewriting traces.
    node["status"] = "complete"
    state = s.state()
    assert state["current"]["status"] == state["nodes"][-1]["status"] == "empty"
    assert node["status"] == "complete"


@pytest.mark.asyncio
async def test_per_call_loom_count_preserves_defaults_and_prefix(session):
    s = session
    root = s.project.add("Source document.", kind="source")
    defaults = s.state()["settings"].copy()
    await s.execute(
        "continue",
        {"node": root["id"], "offset": 6, "branch": True, "count": 5},
        "loom",
    )
    await s.job
    children = s.project.data["nodes"][1:]
    assert len(children) == 5
    assert all(
        n["parent"] == root["id"]
        and n["prompt"] == "Source"
        and n["settings"]["count"] == 5
        for n in children
    )
    assert s.state()["settings"] == defaults
    count = len(s.project.data["nodes"])
    for bad in [0, -2, True, "3", 1.5]:
        with pytest.raises(ValueError, match="count"):
            await s.execute(
                "continue", {"count": bad, "text": "must not save"}, "invalid"
            )
    assert len(s.project.data["nodes"]) == count


@pytest.mark.asyncio
async def test_fork_copies_selected_version_and_draft_without_generation(session):
    s = session
    root = s.project.add("Source 🙂 tail", kind="source")
    unrelated = s.project.add("Unrelated", kind="source")
    await s.execute("node.fork", {"node": root["id"]}, "fork")
    fork = s.project.node(s.project.data["current"])
    assert fork["text"] == root["text"]
    assert fork["parent"] == root["id"] and fork["kind"] == "fork"
    assert fork["id"] != root["id"]
    assert s.project.origins(fork["id"]) == s.project.origins(root["id"])
    await s.execute(
        "node.fork", {"node": fork["id"], "text": "Edited 🙂 tail"}, "draft"
    )
    draft_fork = s.project.node(s.project.data["current"])
    edit = s.project.node(draft_fork["parent"])
    assert edit["parent"] == fork["id"] and edit["kind"] == "edit"
    assert edit["text"] == draft_fork["text"] == "Edited 🙂 tail"
    assert root["text"] == fork["text"] == "Source 🙂 tail"
    assert unrelated["text"] == "Unrelated"
    assert not any(event[0] == "token" for event in s.events)
    assert not s.busy


@pytest.mark.asyncio
async def test_settings_have_no_arbitrary_ceilings(session):
    s = session
    values = dict(count=20, rounds=15, n_predict=32000, temperature=4.5, top_p=0)
    await s.execute(
        "configure", {"settings": values, "model_context": 65536}, "settings"
    )
    assert s.state()["settings"] == values
    assert s.state()["model_context"] == 65536
    await s.execute(
        "configure",
        {"settings": {"n_predict": -1, "temperature": 0}, "model_context": 0},
        "native",
    )
    assert s.state()["settings"]["n_predict"] == -1
    assert s.runtime.model["context"] == 0
    for key, value in [
        ("n_predict", 0),
        ("count", 0),
        ("rounds", True),
        ("temperature", float("nan")),
        ("top_p", 1.1),
    ]:
        with pytest.raises(ValueError):
            s.validate_settings({**values, key: value})


@pytest.mark.asyncio
async def test_per_branch_token_caps_preserve_defaults(session):
    s = session
    root = s.project.add("Source text", kind="source")
    defaults = s.state()["settings"].copy()
    await s.execute(
        "continue",
        {"node": root["id"], "count": 3, "n_predict": 1024},
        "range",
    )
    await s.job
    branches = s.project.data["nodes"][1:]
    assert [n["settings"]["n_predict"] for n in branches] == [1024, 1024, 1024]
    assert all("token_range" not in n["settings"] for n in branches)
    assert s.state()["settings"] == defaults
    before = len(s.project.data["nodes"])
    for bad in [[0, 20], [20, 10], [True, 20], [1], "1-2"]:
        with pytest.raises(ValueError, match="Token range"):
            await s.execute(
                "continue", {"text": "invalid edit", "token_range": bad}, "invalid"
            )
    assert len(s.project.data["nodes"]) == before


@pytest.mark.asyncio
async def test_document_notes_do_not_change_prompt_text(session):
    s = session
    root = s.project.add("First 🙂\nSecond line", kind="source")
    before = root["text"]
    await s.execute(
        "note.add",
        {"node": root["id"], "offset": 8, "note": "Interesting transition"},
        "note",
    )
    assert s.current()["id"] == root["id"]
    assert len(s.project.data["nodes"]) == 1
    assert s.current()["text"] == before
    note = s.project.data["annotations"][-1]
    assert note["node"] == root["id"] and note["start"] == note["end"] == 8
    assert note["note"] == "Interesting transition"
    draft = before + "\nDraft ending"
    await s.execute(
        "note.add",
        {
            "node": root["id"],
            "text": draft,
            "offset": len(draft),
            "note": "Ending note",
        },
        "draft-note",
    )
    edited = s.current()
    assert edited["parent"] == root["id"] and edited["text"] == draft
    assert root["text"] == before
    assert s.project.data["annotations"][-1]["node"] == edited["id"]
    before_count = len(s.project.data["nodes"])
    for invalid in [dict(note=""), dict(note="bad", offset=-1)]:
        with pytest.raises(ValueError):
            await s.execute(
                "note.add",
                {"node": edited["id"], "text": "not saved", **invalid},
                "bad-note",
            )
    assert len(s.project.data["nodes"]) == before_count


@pytest.mark.asyncio
async def test_note_update_preserves_document_and_anchor(session):
    s = session
    root = s.project.add("Document 🙂", kind="source")
    await s.execute(
        "note.add", {"node": root["id"], "offset": 5, "note": "Before"}, "add"
    )
    original = s.project.data["annotations"][0].copy()
    await s.execute(
        "note.update",
        {"node": root["id"], "id": original["id"], "note": "After"},
        "update",
    )
    notes = s.project.data["annotations"]
    assert len(notes) == 1 and notes[0]["note"] == "After"
    for key in ("id", "node", "start", "end", "text_sha256", "created"):
        assert notes[0][key] == original[key]
    assert len(s.project.data["nodes"]) == 1 and root["text"] == "Document 🙂"
    for args in [
        dict(node="wrong", id=original["id"], note="bad"),
        dict(node=root["id"], id=original["id"], note=""),
    ]:
        with pytest.raises(ValueError):
            await s.execute("note.update", args, "bad")
    assert notes[0]["note"] == "After"


@pytest.mark.asyncio
async def test_simulator_commands_and_scoped_grow_settings(session):
    s = session
    node = s.project.add("One path.", kind="source")
    node["kept"] = True
    await s.execute(
        "grow.configure", {"settings": {"count": 2, "rounds": 3}}, "grow-settings"
    )
    assert s.state()["grow_settings"]["rounds"] == 3
    assert s.state()["settings"]["rounds"] == 1
    await s.execute(
        "simulator.configure", {"documents": [node["id"]], "turns": 2}, "config"
    )
    await s.execute("simulator.run", {}, "simulate")
    await s.job
    assert not s.busy
    run = s.project.data["simulation_runs"][0]
    assert run["status"] == "complete"
    await s.execute("simulator.inspect", {"run": run["id"]}, "inspect")
    assert s.events[-1][0] == "inspection"
    assert s.events[-1][1]["conversations"][0]["turns"][1]["trace"]["request"][
        "prompt"
    ].startswith("One path.")
    assert s.state()["simulation_runs"][0]["id"] == run["id"]


@pytest.mark.asyncio
async def test_browse_and_inspect_during_generation_keeps_view(session):
    s = session
    first = s.project.add("Active seed", kind="source")
    other = s.project.add("Read this while waiting", kind="source")
    s.project.data["current"] = first["id"]
    started, finish = asyncio.Event(), asyncio.Event()

    class Slow(FakeRuntime):
        async def stream(self, prompt, settings, trace):
            trace["request"] = {"prompt": prompt}
            yield " Partial"
            started.set()
            await finish.wait()
            yield " end"

    s.runtime = Slow(s.project.folder, DEFAULT_MODEL)
    await s.execute("continue", {"branch": True, "count": 2}, "job")
    await started.wait()
    await s.execute("node.open", {"node": other["id"]}, "browse")
    await s.execute("inspect", {}, "inspect")
    assert s.events[-1][1]["node"]["id"] == other["id"]
    assert s.state()["current"]["id"] == other["id"]
    with pytest.raises(ValueError):
        await s.execute(
            "node.rename", {"node": other["id"], "title": "blocked"}, "edit"
        )
    finish.set()
    await s.job
    assert s.state()["current"]["id"] == other["id"]
    assert s.state()["active_node"] != other["id"]
    assert s.project.node(s.state()["active_node"])["text"] == "Active seed Partial end"


@pytest.mark.asyncio
async def test_read_old_simulation_during_new_run_and_rename(session):
    s = session
    doc = s.project.add("Recognizable first line\nRest of document", kind="source")
    doc["kept"] = True
    await s.execute(
        "simulator.configure", {"documents": [doc["id"]], "turns": 1}, "config"
    )
    await s.execute("simulator.run", {}, "first")
    await s.job
    old_id = s.project.data["simulation_runs"][0]["id"]
    started, finish = asyncio.Event(), asyncio.Event()

    class Slow(FakeRuntime):
        async def stream(self, prompt, settings, trace):
            yield "partial"
            started.set()
            await finish.wait()

    s.runtime_factory = Slow
    await s.execute("simulator.run", {}, "next")
    await started.wait()
    await s.execute("simulator.open", {"run": old_id}, "view")
    assert s.events[-1][1]["opened"] and s.events[-1][1]["id"] == old_id
    await s.execute("simulator.inspect", {"run": old_id}, "inspect")
    assert s.events[-1][1]["id"] == old_id
    assert s.state()["simulation_runs"][0]["count"] == 1
    await s.execute("cancel", {}, "cancel")
    await s.execute("node.rename", {"node": doc["id"], "title": "My path"}, "title")
    assert s.state()["nodes"][0]["title"] == "My path"
    assert doc["text"].startswith("Recognizable first line")


@pytest.mark.asyncio
async def test_simulator_run_overrides_are_recorded_not_saved(session):
    s = session
    doc = s.project.add("Paths", kind="source")
    doc["kept"] = True
    await s.execute(
        "simulator.configure",
        {"documents": [doc["id"]], "turns": 2, "opening_mode": "generated"},
        "config",
    )
    saved = json.loads(json.dumps(s.project.data["simulator_config"]))
    await s.execute(
        "simulator.run",
        {
            "count": 3,
            "n_predict": 32,
            "turns": 3,
            "message": "What makes a path yours?",
        },
        "loom",
    )
    await s.job
    run = s.project.data["simulation_runs"][-1]
    assert len(run["conversations"]) == 3
    assert run["config"]["turns"] == 3
    assert s.project.data["simulator_config"] == saved
    for conversation in run["conversations"]:
        assert len(conversation["turns"]) == 6
        assert conversation["turns"][0]["text"] == "What makes a path yours?"
        assert run["config"]["opening_mode"] == "fixed"
        for turn in conversation["turns"][1:]:
            assert turn["settings"]["n_predict"] == 32
            assert "token_range" not in turn["settings"]
    for overrides in [
        {"count": 0},
        {"message": " "},
        {"message": 3},
        {"turns": 0},
        {"n_predict": 0},
        {"token_range": [40, 10]},
        {"token_range": [0, 2]},
        {"token_range": [True, 4]},
    ]:
        with pytest.raises(ValueError):
            await s.execute("simulator.run", overrides, "invalid")
    assert len(s.project.data["simulation_runs"]) == 1


@pytest.mark.asyncio
async def test_opening_preview_does_not_change_configuration(session):
    from character_lab import simulator

    before = simulator.configuration(session.project, session.runtime.model["alias"])
    await session.execute("simulator.preview", {}, "preview")
    await session.job
    run = session.project.data["simulation_runs"][-1]
    assert run["preview"] and len(run["conversations"]) == 3
    assert all(len(c["turns"]) == 1 for c in run["conversations"])
    assert (
        simulator.configuration(session.project, session.runtime.model["alias"])
        == before
    )


@pytest.mark.asyncio
async def test_cancel_parallel_branches_preserves_every_partial(session):
    started = asyncio.Event()
    count = 0

    class SlowBatch(FakeRuntime):
        async def stream(self, prompt, settings, trace):
            nonlocal count
            count += 1
            yield "partial"
            if count == 4:
                started.set()
            await asyncio.sleep(60)

    s = session
    s.runtime = SlowBatch(s.project.folder, s.runtime.model)
    root = s.project.add("Source", kind="source")
    await s.execute("continue", {"node": root["id"], "count": 8}, "batch")
    await started.wait()
    task = s.job
    task.cancel()
    await task
    branches = s.project.data["nodes"][1:]
    assert len(branches) == 4  # untouched queue items don't become fake output
    assert all(
        n["status"] == "stopped" and n["text"] == "Sourcepartial" for n in branches
    )
    assert not s.busy


@pytest.mark.asyncio
async def test_loom_policy_crud_and_default_protection(session):
    from character_lab import simulator

    s = session
    await s.execute(
        "loom-policy.add",
        {"name": "Drift", "spec": "Is the speaker abandoning its voice?"},
        "add",
    )
    config = simulator.configuration(s.project, s.runtime.model["alias"])
    custom = config["monitor_dimensions"][-1]
    assert custom["action"] == "warn"
    await s.execute(
        "loom-policy.update", {"id": custom["id"], "action": "stop"}, "edit"
    )
    with pytest.raises(ValueError, match="cannot be deleted"):
        await s.execute("loom-policy.delete", {"id": "looping"}, "delete")
    await s.execute("loom-policy.delete", {"id": custom["id"]}, "delete")
    assert len(s.project.data["simulator_config"]["monitor_dimensions"]) == 3


@pytest.mark.asyncio
async def test_simulator_fork_and_resume_commands(session):
    s = session
    doc = s.project.add("Frozen anthology.", kind="source")
    doc["kept"] = True
    await s.execute(
        "simulator.configure", {"documents": [doc["id"]], "turns": 1}, "config"
    )
    await s.execute("simulator.run", {}, "original")
    await s.job
    source = s.project.data["simulation_runs"][-1]
    await s.execute("simulator.fork", {"run": source["id"], "conversation": 0}, "fork")
    fork = s.project.data["simulation_runs"][-1]
    assert fork["status"] == "draft"
    doc["text"] = "Changed anthology"
    await s.execute(
        "simulator.run",
        {"run": fork["id"], "conversation": 0, "count": 2, "turns": 2},
        "resume",
    )
    await s.job
    run = s.project.data["simulation_runs"][-1]
    assert run["status"] == "complete"
    assert len(run["conversations"]) == 2
    assert len(run["conversations"][0]["turns"]) == 6
    assert run["documents"][0]["text"] == "Frozen anthology."
    assert source["conversations"][0]["turns"] == fork["conversations"][0]["turns"]
    before = len(s.project.data["simulation_runs"])
    with pytest.raises(ValueError, match="Clear the conversation selection"):
        await s.execute(
            "simulator.run",
            {"run": fork["id"], "conversation": 0, "message": "Replace history?"},
            "invalid-message",
        )
    assert len(s.project.data["simulation_runs"]) == before

    with pytest.raises(ValueError, match="Conversation not found"):
        await s.execute(
            "simulator.open", {"run": source["id"], "conversation": 100}, "invalid"
        )


@pytest.mark.asyncio
async def test_import_refreshes_shared_library_without_selecting_seed(
    session, tmp_path
):
    s = session
    path = tmp_path / "Imported.md"
    path.write_text("A new seed.\n")
    selected = list(s.project.selected)
    await s.execute("library.import", {"path": str(path)}, "import")
    assert s.project.selected == selected
    assert any(source["title"] == "Imported" for source in s.sources)
    assert [event[0] for event in s.events[-3:]] == [
        "library",
        "state",
        "library.imported",
    ]


async def test_edit_state_acknowledges_its_request(session):
    root = session.project.add("original")
    await session.execute(
        "node.edit", {"node": root["id"], "text": "edited 🙂"}, "save-42"
    )
    kind, state, request_id = session.events[-1]
    assert (kind, request_id) == ("state", "save-42")
    assert state["current"]["text"] == "edited 🙂"
    assert state["current"]["parent"] == root["id"]


@pytest.mark.asyncio
async def test_document_loops_share_batch_generation_and_preserve_ancestry(session):
    s = session
    root = s.project.add("A source.", kind="source")
    defaults = s.state()["settings"].copy()
    await s.execute(
        "continue",
        {"node": root["id"], "count": 2, "loops": 3, "n_predict": 32},
        "loops",
    )
    await s.job
    nodes = s.project.data["nodes"][1:]
    assert len(nodes) == 6
    policy = s.project.data["policy_runs"][-1]
    assert policy["status"] == "complete" and len(policy["steps"]) == 3
    for previous, current in zip(policy["steps"], policy["steps"][1:]):
        winner = s.project.node(previous["decision"]["selected"])
        assert all(
            s.project.node(key)["parent"] == winner["id"]
            for key in current["candidates"]
        )
        assert current["messages"][0]["content"] == policy["prompt"]
    assert not any(n.get("kept") for n in nodes)
    assert all(n["settings"]["n_predict"] == 32 for n in nodes)
    assert s.state()["settings"] == defaults and not s.busy


@pytest.mark.asyncio
async def test_simulator_loops_advance_selected_transcript_not_original_seed(session):
    s = session
    doc = s.project.add("An anthology.", kind="source")
    doc["kept"] = True
    await s.execute("simulator.configure", {"documents": [doc["id"]]}, "config")
    await s.execute(
        "simulator.run", {"count": 2, "turns": 2, "loops": 3, "n_predict": 16}, "loops"
    )
    await s.job
    runs = s.project.data["simulation_runs"]
    assert len(runs) == 3
    assert all(len(r["conversations"]) == 2 for r in runs)
    assert runs[1]["parent"] == {"run": runs[0]["id"], "conversation": 0}
    assert runs[2]["parent"] == {"run": runs[1]["id"], "conversation": 0}
    final = runs[-1]["conversations"][0]
    assert sum(t["role"] == "character" for t in final["turns"]) == 6
    assert s.project.data["policy_runs"][-1]["status"] == "complete"
    assert not s.busy


@pytest.mark.asyncio
async def test_loops_validate_selector_before_generating(session):
    s = session
    s.project.add("Source", kind="source")
    s.policy_model["path"] = ""
    before = len(s.project.data["nodes"])
    with pytest.raises(ValueError, match="selection model"):
        await s.execute("continue", {"count": 2, "loops": 2}, "invalid")
    assert len(s.project.data["nodes"]) == before and not s.busy
    with pytest.raises(ValueError, match="Loops"):
        await s.execute("continue", {"loops": 0}, "invalid")


@pytest.mark.asyncio
async def test_no_selection_does_not_start_an_extra_loop(session, monkeypatch):
    async def none(self, messages, trace):
        data = json.loads(messages[1]["content"])
        return dict(
            reviews=[
                dict(
                    node=c["node"],
                    decision="pass",
                    reason="Not useful",
                    evidence=c["continuation"],
                )
                for c in data["candidates"]
            ],
            selected=None,
            reason="No eligible result",
        )

    monkeypatch.setattr(FakeRuntime, "judge", none)
    session.project.add("Source", kind="source")
    await session.execute("continue", {"count": 2, "loops": 4}, "loops")
    await session.job
    assert len(session.project.data["nodes"]) == 3
    assert session.project.data["policy_runs"][-1]["status"] == "no_selection"


@pytest.mark.asyncio
async def test_cancel_during_selection_preserves_batch_and_trace(session, monkeypatch):
    async def cancelled(self, messages, trace):
        trace["request"] = messages
        raise asyncio.CancelledError()

    monkeypatch.setattr(FakeRuntime, "judge", cancelled)
    session.project.add("Seed", kind="source")
    await session.execute("continue", {"count": 2, "loops": 3}, "loops")
    await session.job
    loaded = Project(session.project.folder)
    run = loaded.data["policy_runs"][-1]
    assert run["status"] == "stopped" and len(run["steps"]) == 1
    assert len(run["steps"][0]["trace"]["request"]) == 2
    assert all(n["status"] == "complete" for n in loaded.data["nodes"])
    assert not session.busy


@pytest.mark.asyncio
async def test_document_monitor_stop_preserves_output_and_never_selects(
    session, monkeypatch
):
    monkeypatch.setenv("OPENROUTER_API_KEY", "test-only")
    from character_lab import monitor

    async def scan(config, conversation, turn):
        turn["monitor"] = dict(
            status="complete",
            detections=[
                dict(action="stop", name="Looping", color="amber", confidence=0.99)
            ],
        )

    monkeypatch.setattr(monitor, "scan", scan)
    session.project.add("Seed", kind="source")
    await session.execute("simulator.configure", {"monitor_mode": "jev"}, "config")
    await session.execute("continue", {"count": 1, "loops": 1}, "loom")
    await session.job
    node = session.project.data["nodes"][-1]
    assert node["status"] == "policy_stopped"
    assert node["monitor_checks"][0]["detections"][0]["action"] == "stop"
    assert node["text"].startswith("Seed")
    assert not session.project.data.get("policy_runs")


@pytest.mark.asyncio
async def test_monitor_key_setup_enables_without_exposing_secret(session, monkeypatch):
    from character_lab import credentials

    monkeypatch.delenv("OPENROUTER_API_KEY", raising=False)
    assert (
        session.project.data.get("simulator_config", {}).get("monitor_mode", "off")
        == "off"
    )
    with pytest.raises(ValueError, match="API key"):
        await session.execute("simulator.configure", {"monitor_mode": "jev"}, "enable")
    key = "fake-secret-for-test"
    await session.execute("loom-policy.key", {"key": key}, "key")
    assert session.project.data["simulator_config"]["monitor_mode"] == "jev"
    assert credentials.openrouter_key() == (key, "saved")
    assert key not in json.dumps(session.events)
    assert key not in session.project.path.read_text()
    state = [data for kind, data, _ in session.events if kind == "state"][-1]
    assert state["monitor_key_source"] == "saved"
    await session.execute("simulator.configure", {"monitor_mode": "off"}, "off")
    assert credentials.openrouter_key() == (key, "saved")


@pytest.mark.asyncio
async def test_create_behavior_with_full_configuration(session):
    values = {
        "name": "Voice drift",
        "spec": "Flag loss of voice.\n\nIgnore quoted speech.",
        "enabled": False,
        "action": "stop",
        "decision": "threshold",
        "threshold": 0.9,
        "color": "violet",
    }
    await session.execute("loom-policy.add", values, "create")
    item = session.project.data["simulator_config"]["monitor_dimensions"][-1]
    assert all(item[key] == value for key, value in values.items())
    with pytest.raises(ValueError):
        await session.execute("loom-policy.add", values | {"threshold": 2}, "bad")
    assert len(session.project.data["simulator_config"]["monitor_dimensions"]) == 4
