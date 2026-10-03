"""Read-only inspection evidence, linked by saved identity rather than UI state.

Raw records remain intact. Ancestor generation evidence is explicitly identified;
its judgments are not attributed to a later edit of that document.
"""

import copy

from . import exploration


def selection_runs(project, candidates=(), policy_ids=()):
    runs = project.data.get("policy_runs", [])
    candidates, linked = set(candidates), set(policy_ids)
    linked.update(
        run["id"]
        for run in runs
        if any(
            candidates.intersection(step.get("candidates", []))
            for step in run.get("steps", [])
        )
    )
    # Retries preserve frozen candidates. Include the whole attempt family even
    # for historical records whose steps did not repeat membership metadata.
    changed = True
    while changed:
        before = set(linked)
        for run in runs:
            if run["id"] in linked and run.get("retry_of"):
                linked.add(run["retry_of"])
            if run.get("retry_of") in linked:
                linked.add(run["id"])
        changed = linked != before
    return [run for run in runs if run["id"] in linked]


def evaluations(project, matches):
    records = [
        record
        for record in project.data.get("evaluations", [])
        if matches(record.get("target", {}))
    ]
    ids = {record["id"] for record in records}
    return dict(
        evaluations=records,
        evaluation_runs=[
            run
            for run in project.data.get("evaluation_runs", [])
            if ids.intersection(run.get("records", []))
        ],
    )


def document(project, node):
    ancestor = node
    while not ancestor.get("trace") and ancestor.get("parent"):
        ancestor = project.node(ancestor["parent"])
    groups = [
        group["id"]
        for group in project.data.get("document_sets", [])
        if node["id"] in group.get("members", [])
    ]
    record = dict(
        kind="document",
        node=node,
        generation=ancestor,
        generation_inherited=ancestor["id"] != node["id"],
        generation_source_id=ancestor["id"],
        origins=project.origins(node["id"]),
        selection_runs=selection_runs(
            project,
            [node["id"], *groups],
            [node["policy_run"]] if node.get("policy_run") else [],
        ),
        annotations=[
            a for a in project.data.get("annotations", []) if a["node"] == node["id"]
        ],
        **evaluations(project, lambda target: target.get("node") == node["id"]),
    )
    record["selection_summaries"] = [
        exploration.summary(run) for run in record["selection_runs"]
    ]
    return copy.deepcopy(record)


def simulation(project, run, conversation=None):
    if conversation is not None and (
        type(conversation) is not int
        or not 0 <= conversation < len(run["conversations"])
    ):
        raise ValueError("Conversation not found")
    record = dict(run)
    linked = selection_runs(
        project, [run["id"]], [run["policy_run"]] if run.get("policy_run") else []
    )
    record.update(
        kind="simulation",
        selection=next((r for r in linked if r["id"] == run.get("policy_run")), None),
        selection_runs=linked,
        **evaluations(
            project,
            lambda target: (
                target.get("run") == run["id"]
                and (conversation is None or target.get("conversation") == conversation)
            ),
        ),
    )
    if conversation is not None:
        record["inspected_conversation"] = conversation
        record["conversations"] = [run["conversations"][conversation]]
    record["selection_summaries"] = [
        exploration.summary(run) for run in record["selection_runs"]
    ]
    return copy.deepcopy(record)


def selection(project, run_id):
    run = next(
        run for run in project.data.get("policy_runs", []) if run["id"] == run_id
    )
    return copy.deepcopy(run) | {
        "kind": "selection",
        "selection_summaries": [exploration.summary(run)],
    }
