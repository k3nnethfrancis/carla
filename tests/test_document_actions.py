"""Action semantics: immutable evidence, logical advancement and set shape."""

import json
from copy import deepcopy

import pytest

from character_lab import document_actions as actions
from character_lab.domain import Project


def generated(project, text):
    return project.add(text, kind="generated")


def run(project, action, ids=None, **kwargs):
    plan = actions.plan(project, action, ids, **kwargs)
    return [
        actions.create_revision(project, candidate)
        for candidate in actions.begin(project, plan)
    ]


def test_continue_preserves_evidence_and_advances_identity(tmp_path):
    project = Project(tmp_path)
    original = generated(project, "One path.")
    project.keep(original["id"])
    before = deepcopy(original)
    continued = run(project, "continue", [original["id"]])[0]
    continued["text"] += " Another path."
    project.save()

    assert original == before
    assert continued["document_id"] == original["document_id"]
    assert continued["revision_of"] == original["id"]
    assert continued["parent"] == original["id"]
    assert continued["kept"] is False
    assert project.data["document_heads"][original["document_id"]] == continued["id"]
    reopened = Project(tmp_path)
    assert reopened.node(original["id"]) == before
    assert reopened.data["document_heads"] == project.data["document_heads"]


def test_continue_old_version_or_cursor_branches_without_replacing_future(tmp_path):
    project = Project(tmp_path)
    first = generated(project, "Past. Future.")
    latest = run(project, "continue", [first["id"]])[0]
    latest["text"] += " New future."
    project.save()
    historical = run(project, "continue", [first["id"]])[0]
    anchored = run(project, "continue", [latest["id"]], offsets={latest["id"]: 5})[0]

    assert anchored["text"] == "Past."
    assert (
        len({first["document_id"], historical["document_id"], anchored["document_id"]})
        == 3
    )
    assert project.data["document_heads"][first["document_id"]] == latest["id"]
    assert (
        historical["branch_reason"] == anchored["branch_reason"] == "historical_prefix"
    )
    assert first["text"] == "Past. Future."
    assert latest["text"] == "Past. Future. New future."


def test_source_always_remains_source_identity(tmp_path):
    project = Project(tmp_path)
    source = project.add("A source passage", kind="source")
    result = run(project, "continue", [source["id"]])[0]
    assert result["document_id"] != source["document_id"]
    assert result["branch_reason"] == "source"
    assert project.data["document_heads"][source["document_id"]] == source["id"]


def test_loom_preserves_sets_and_input_order_despite_completion_order(tmp_path):
    project = Project(tmp_path)
    originals = [generated(project, word) for word in ["A", "B", "C", "D"]]
    plan = actions.plan(project, "loom", [n["id"] for n in originals], count=3)
    candidates = actions.begin(project, plan)
    assert len(candidates) == 12
    for candidate in reversed(candidates):
        actions.create_revision(project, candidate)

    groups = project.data["document_sets"]
    assert len(groups) == 3
    assert all(len(group["members"]) == 4 for group in groups)
    for group in groups:
        members = [project.node(key) for key in group["members"]]
        assert [node["text"] for node in members] == ["A", "B", "C", "D"]
        assert [node["parent"] for node in members] == [n["id"] for n in originals]
        assert all(
            node["document_id"] != original["document_id"]
            for node, original in zip(members, originals)
        )


def test_whole_set_continue_revises_membership_without_rewriting_old_set(tmp_path):
    project = Project(tmp_path)
    originals = [generated(project, word) for word in ["A", "B"]]
    first = run(project, "loom", [n["id"] for n in originals])
    source_set = deepcopy(project.data["document_sets"][-1])
    advanced = run(project, "continue", set_id=source_set["id"])
    current_set = project.data["document_sets"][-1]

    assert actions.group(project, source_set["id"]) == source_set
    assert current_set["id"] != source_set["id"]
    assert current_set["set_id"] == source_set["set_id"]
    assert current_set["previous_revision"] == source_set["id"]
    assert current_set["members"] == [n["id"] for n in advanced]
    assert [n["document_id"] for n in first] == [n["document_id"] for n in advanced]
    assert project.data["document_set_heads"][source_set["set_id"]] == current_set["id"]


def test_branch_copies_set_without_generation_or_keep_inheritance(tmp_path):
    project = Project(tmp_path)
    originals = [generated(project, word) for word in ["A", "B"]]
    project.keep(originals[0]["id"])
    plan = actions.plan(project, "branch", [n["id"] for n in originals])
    branches = actions.branch(project, plan)
    assert [n["text"] for n in branches] == ["A", "B"]
    assert all(n["kind"] == "fork" and not n["kept"] for n in branches)
    assert all("settings" not in n and "trace" not in n for n in branches)
    assert [n["parent"] for n in branches] == [n["id"] for n in originals]
    assert [project.origins(n["id"]) for n in branches] == [
        project.origins(n["id"]) for n in originals
    ]


def test_legacy_nodes_gain_identity_without_ancestry_reinterpretation(tmp_path):
    legacy = dict(
        version=1,
        selected=[],
        current="child",
        snapshots=[],
        nodes=[
            dict(id="root", text="Old.", parent=None, kind="source", kept=False),
            dict(
                id="child", text="Old. New.", parent="root", kind="generated", kept=True
            ),
        ],
    )
    (tmp_path / "project.json").write_text(json.dumps(legacy))
    project = Project(tmp_path)
    assert project.data["document_heads"] == {"root": "root", "child": "child"}
    assert project.node("child")["kept"]
    assert project.node("child")["parent"] == "root"
    assert "revision_of" not in project.node("child")


def test_validation_has_no_saved_side_effects(tmp_path):
    project = Project(tmp_path)
    node = generated(project, "αβ")
    before = deepcopy(project.data)
    for kwargs in [
        {"count": 0},
        {"count": True},
        {"count": 2},
        {"offsets": {node["id"]: -1}},
        {"offsets": {node["id"]: 3}},
        {"offsets": {"missing": 1}},
        {"offsets": []},
    ]:
        with pytest.raises(ValueError):
            actions.plan(project, "continue", [node["id"]], **kwargs)
    with pytest.raises(ValueError, match="twice"):
        actions.plan(project, "loom", [node["id"], node["id"]])
    with pytest.raises(ValueError, match="no longer exists"):
        actions.plan(project, "loom", ["missing"])
    assert project.data == before


def test_deleting_latest_revision_restores_surviving_head(tmp_path):
    project = Project(tmp_path)
    original = generated(project, "A")
    latest = run(project, "continue", [original["id"]])[0]
    project.delete_nodes([latest["id"]], [latest["id"]])
    assert project.data["document_heads"][original["document_id"]] == original["id"]
    with pytest.raises(ValueError, match="no longer exists"):
        actions.plan(project, "continue", set_id=latest["document_set"])
