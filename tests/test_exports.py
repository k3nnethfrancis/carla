"""Exports preserve selected content and evidence without choosing training rules."""

import json

import pytest

from character_lab.domain import Project
from character_lab.exports import export


def test_document_export_keeps_exact_text_ancestry_and_scope(tmp_path):
    p = Project(tmp_path)
    root = p.add("Seed 🙂\n", kind="source")
    node = p.add("Seed 🙂\nNew text", parent=root["id"], settings={"seed": 17})
    p.add("Unrelated", kind="source")
    p.data["annotations"] = [
        dict(id="note", node=node["id"], note="Keep this nuance"),
        dict(id="private", node="other", note="Unrelated note"),
    ]
    before = json.dumps(p.data)
    path = export(p, [], {"scope": "branches", "nodes": [node["id"]]})
    data = json.loads((path / "manifest.json").read_text())
    assert (path / "0001.txt").read_text() == node["text"]
    assert len(data["items"]) == 1
    assert data["items"][0]["document"]["settings"] == {"seed": 17}
    assert [n["id"] for n in data["document_ancestors"]] == [root["id"]]
    assert [a["id"] for a in data["annotations"]] == ["note"]
    assert json.dumps(p.data) == before


def test_conversation_export_preserves_roles_settings_and_member_selection(tmp_path):
    p = Project(tmp_path)
    p.data["simulation_runs"] = [
        dict(
            id="run",
            config={"character_alias": "base"},
            documents=[{"text": "source"}],
            revisions=[{"old": True}],
            conversations=[
                dict(
                    index=0,
                    turns=[
                        dict(role="visitor", text="Question"),
                        dict(role="character", text="Answer"),
                    ],
                ),
                dict(index=1, turns=[dict(role="visitor", text="Other")]),
            ],
        )
    ]
    path = export(
        p, [], {"scope": "simulator", "targets": [{"run": "run", "conversation": 0}]}
    )
    item = json.loads((path / "manifest.json").read_text())["items"][0]
    assert len(item["conversation"]["turns"]) == 2
    assert item["run"]["config"]["character_alias"] == "base"
    assert "revisions" not in item["run"]
    assert "Other" not in (path / "0001.txt").read_text()


def test_evaluation_export_does_not_filter_out_nontraining_items(tmp_path):
    p = Project(tmp_path)
    p.data["evaluation_sets"] = [
        dict(
            id="e",
            name="Example",
            judges=[],
            items=[
                dict(
                    id="i",
                    text="Frozen content",
                    training=False,
                    judgments=["j"],
                    evidence=[{"reason": "Example"}],
                )
            ],
        )
    ]
    p.data["evaluations"] = [dict(id="j", passed=False)]
    path = export(p, [], {"scope": "evaluate", "collection": "e"})
    item = json.loads((path / "manifest.json").read_text())["items"][0]
    assert item["item"]["training"] is False
    assert item["judgments"] == [{"id": "j", "passed": False}]
    assert item["item"]["evidence"]


def test_stale_selection_rejected_before_creating_files(tmp_path):
    p = Project(tmp_path)
    with pytest.raises(ValueError, match="no longer available"):
        export(p, [], {"scope": "branches", "nodes": ["missing"]})
    assert not (tmp_path / "exports").exists()


def test_document_export_retains_frozen_set_shape(tmp_path):
    from character_lab import document_actions

    p = Project(tmp_path)
    a, b = p.add("A"), p.add("B")
    leaves = document_actions.branch(
        p, document_actions.plan(p, "branch", [a["id"], b["id"]])
    )
    folder = export(p, [], {"scope": "branches", "nodes": [leaves[0]["id"]]})
    data = json.loads((folder / "manifest.json").read_text())
    assert len(data["items"]) == 1
    assert data["document_sets"][0]["members"] == [n["id"] for n in leaves]
