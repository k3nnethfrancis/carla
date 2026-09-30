from character_lab import document_actions
from character_lab.document_names import assign_labels
from character_lab.domain import Project


def test_loom_names_share_operation_and_ignore_completion_order(tmp_path):
    p = Project(tmp_path)
    root = p.add("Seed", kind="source")
    plans = document_actions.begin(
        p, document_actions.plan(p, "loom", [root["id"]], count=2)
    )
    second = document_actions.create_revision(p, plans[1])
    first = document_actions.create_revision(p, plans[0])
    assert first["label"] == "branch-1-loom-1-doc-1"
    assert second["label"] == "branch-2-loom-1-doc-1"
    groups = p.data["document_sets"]
    assert groups[0]["operation_id"] == groups[1]["operation_id"]
    assert groups[0]["operation_label"] == "loom-1-doc-1"
    continued = document_actions.create_revision(
        p,
        document_actions.begin(p, document_actions.plan(p, "continue", [first["id"]]))[
            0
        ],
    )
    assert continued["label"] == "continue-1-branch-1-loom-1-doc-1"
    assert Project(tmp_path).node(continued["id"])["label"] == continued["label"]


def test_direct_and_group_operations_share_parent_counter(tmp_path):
    p = Project(tmp_path)
    root = p.add("Seed", kind="source")
    first = p.add("Seed one", parent=root["id"])
    candidate = document_actions.begin(
        p, document_actions.plan(p, "continue", [root["id"]])
    )[0]
    second = document_actions.create_revision(p, candidate)
    assert first["label"] == "continue-1-doc-1"
    assert second["label"] == "continue-2-doc-1"
    p.delete_nodes([second["id"]], [second["id"]])
    p = Project(tmp_path)
    assert p.add("Seed three", parent=root["id"])["label"] == "continue-3-doc-1"


def test_migration_retains_content_custom_titles_and_ids():
    data = {
        "nodes": [
            {
                "id": "root",
                "parent": None,
                "kind": "source",
                "text": "Seed",
                "label": "paths-branch-0001",
            },
            {
                "id": "child",
                "parent": "root",
                "kind": "generated",
                "text": "Seed output",
                "title": "My name",
                "label": "paths-gen-0009",
            },
        ]
    }
    assign_labels(data)
    assert data["nodes"][1]["label"] == "continue-1-doc-1"
    assert data["nodes"][1]["legacy_label"] == "paths-gen-0009"
    assert data["nodes"][1]["title"] == "My name"
    assert data["nodes"][1]["text"] == "Seed output"
    assert data["nodes"][1]["parent"] == "root"
    before = [n["label"] for n in data["nodes"]]
    assign_labels(data)
    assert before == [n["label"] for n in data["nodes"]]
