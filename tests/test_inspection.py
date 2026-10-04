"""Inspection links evidence without changing workspace data or its provenance."""

import copy
from types import SimpleNamespace

import pytest

from character_lab import inspection


def workspace():
    nodes = {
        "generated": dict(
            id="generated",
            trace={"events": [{"content": "hello"}]},
            policy_run="original",
        ),
        "edited": dict(id="edited", parent="generated"),
        "other": dict(id="other"),
    }
    data = dict(
        nodes=nodes,
        document_sets=[dict(id="set", members=["generated", "other"])],
        policy_runs=[
            dict(
                id="original",
                steps=[dict(candidates=["set"], loop=1, status="complete")],
                trace={"raw_response": "judge"},
            ),
            dict(id="retry", retry_of="original", steps=[]),
            dict(
                id="unrelated",
                steps=[dict(candidates=["different"], loop=1, status="complete")],
            ),
        ],
        evaluations=[
            dict(id="doc-result", target={"node": "generated"}),
            dict(id="convo-result", target={"run": "sim", "conversation": 1}),
            dict(id="other-result", target={"run": "sim", "conversation": 0}),
        ],
        evaluation_runs=[dict(id="batch", records=["doc-result", "convo-result"])],
        annotations=[dict(node="edited", text="note")],
    )
    return SimpleNamespace(data=data, node=nodes.__getitem__, origins=lambda key: [key])


def test_document_links_set_selection_retries_and_exact_evaluations():
    project = workspace()
    before = copy.deepcopy(project.data)
    record = inspection.document(project, project.node("generated"))
    assert [r["id"] for r in record["selection_runs"]] == ["original", "retry"]
    assert [r["id"] for r in record["evaluations"]] == ["doc-result"]
    assert record["evaluation_runs"][0]["id"] == "batch"
    assert record["selection_summaries"][0]["steps"][0]["candidates"] == ["set"]
    assert record["generation_inherited"] is False
    assert record["node"]["trace"]["events"] == [{"content": "hello"}]
    # Membership alone works when the node has no explicit policy-run pointer.
    assert (
        len(inspection.document(project, project.node("other"))["selection_runs"]) == 2
    )
    record["node"]["trace"]["events"].clear()
    assert project.data == before


def test_edit_does_not_inherit_ancestor_judgments():
    project = workspace()
    record = inspection.document(project, project.node("edited"))
    assert record["generation_inherited"] is True
    assert record["generation_source_id"] == "generated"
    assert record["selection_runs"] == []
    assert record["evaluations"] == []
    assert record["annotations"] == [dict(node="edited", text="note")]
    assert record["generation"]["trace"]["events"]


def test_simulation_inspection_scopes_conversation_and_preserves_raw_evidence():
    project = workspace()
    run = dict(
        id="sim",
        policy_run="original",
        conversations=[dict(turns=[]), dict(turns=[dict(trace={"events": [1, 2]})])],
    )
    before = copy.deepcopy(run)
    record = inspection.simulation(project, run, 1)
    assert record["inspected_conversation"] == 1
    assert record["conversations"] == [run["conversations"][1]]
    assert [r["id"] for r in record["evaluations"]] == ["convo-result"]
    assert [r["id"] for r in record["selection_runs"]] == ["original", "retry"]
    record["conversations"][0]["turns"].clear()
    assert run == before
    assert len(inspection.simulation(project, run)["evaluations"]) == 2
    for invalid in [-1, 2, True, "1"]:
        with pytest.raises(ValueError, match="Conversation not found"):
            inspection.simulation(project, run, invalid)
