"""Resolve Simulator actions without UI state or inference side effects.

A run is a set of conversations. Alternative groups relate new runs without
flattening their members. Continued heads keep their IDs and archive the exact
previous run version before any streaming mutation.
"""

import copy
import uuid

from . import action_scope
from .domain import now


def seed_sets(seed):
    return seed.get("sets", [seed]) if seed else [None]


def members(seed):
    return seed.get("conversations", [seed]) if seed else []


def resolve_seed(project, args):
    from .simulator import batch_seed, conversation_seed

    scope = args.get("scope")
    if scope is not None:
        if any(
            key in args for key in ("targets", "run", "conversation", "conversations")
        ):
            raise ValueError("Supply one explicit conversation scope")
        scope = action_scope.validate(scope, "conversation")
        targets = list(action_scope.leaves(scope))
        if any(t.get("kind") != "conversation" for t in targets):
            raise ValueError("Simulator needs conversation targets")
    elif "targets" in args:
        targets = args["targets"]
    else:
        targets = None
    if targets is not None:
        if not isinstance(targets, list) or not targets:
            raise ValueError("Select at least one conversation")
        grouped = {}
        for target in targets:
            if not isinstance(target, dict) or not isinstance(target.get("run"), str):
                raise ValueError("Supply conversation targets with a run and index")
            index = target.get("conversation")
            if type(index) is not int:
                raise ValueError("Supply a conversation index")
            indices = grouped.setdefault(target["run"], [])
            if index in indices:
                raise ValueError("Select unique conversation targets")
            indices.append(index)
        sets = [batch_seed(project, run, indices) for run, indices in grouped.items()]
        result = sets[0] if len(sets) == 1 else {"sets": sets}
        if scope and scope["kind"] == "conversation":
            result = sets[0]["conversations"][0]
        result["scope"] = scope or scope_for_sets(sets)
        return result
    if "run" not in args:
        return None
    result = (
        conversation_seed(project, args["run"], args["conversation"])
        if "conversation" in args
        else batch_seed(project, args["run"], args.get("conversations"))
    )
    result["scope"] = scope_for_sets([result])
    return result


def add_visitor(seed, message):
    """Validate the complete selection before editing even the frozen seed."""
    if not isinstance(message, str) or not message.strip():
        raise ValueError("Supply a nonempty visitor message")
    leaves = [leaf for item in seed_sets(seed) for leaf in members(item)]
    if any(
        leaf["turns"] and leaf["turns"][-1]["role"] != "character" for leaf in leaves
    ):
        raise ValueError(
            "A selected conversation has an unanswered visitor message; continue its reply or edit that message first"
        )
    result = copy.deepcopy(seed)
    for item in seed_sets(result):
        for leaf in members(item):
            leaf["turns"].append(
                dict(role="visitor", text=message, origin="human", status="complete")
            )
    return result


def validate_seed(project, seed):
    """Frozen targets must still be the heads that the user selected."""
    for item in seed_sets(seed):
        for leaf in members(item):
            run = next(
                (
                    r
                    for r in project.data.get("simulation_runs", [])
                    if r["id"] == leaf["parent"]["run"]
                ),
                None,
            )
            if run is None or run.get("revision", 0) != leaf.get("source_revision", 0):
                raise ValueError(
                    "The selected conversation changed; select its current version again"
                )
            if run["status"] == "running":
                raise ValueError("Stop the active run before branching or continuing")


def archive(run):
    snapshot = copy.deepcopy({k: v for k, v in run.items() if k != "revisions"})
    run.setdefault("revisions", []).append(snapshot)
    run["revision"] = run.get("revision", 0) + 1
    return snapshot


def continue_run(project, config, seed):
    """All validation precedes this commit; untouched siblings retain exact content."""
    parent = seed["parent"]
    run = next(r for r in project.data["simulation_runs"] if r["id"] == parent["run"])
    archive(run)
    selected = []
    for leaf in members(seed):
        conversation = run["conversations"][leaf["parent"]["conversation"]]
        conversation["turns"] = copy.deepcopy(leaf["turns"])
        conversation["revision"] = conversation.get("revision", 0) + 1
        conversation["status"] = "queued"
        selected.append(conversation)
    run.update(
        status="running", config=copy.deepcopy(config), updated=now(), action="continue"
    )
    run["config"]["conversations"] = len(run["conversations"])
    run.pop("error", None)
    run.pop("finished", None)
    run.pop("policy_run", None)
    run.pop("loop", None)
    run["affected_conversations"] = [c["index"] for c in selected]
    return run, selected


def candidate_sets(runs):
    """Selection judges alternatives at set scope, never silently picks a leaf."""
    grouped = {}
    for run in runs:
        grouped.setdefault(str(run.get("alternative_index", 0)), []).append(run)
    return grouped


def group_candidates(runs):
    if len(runs) == 1 and "alternative_group" not in runs[0]:
        return [
            dict(
                id=str(c["index"]),
                prompt="",
                text="\n\n".join(f"{t['role']}: {t['text']}" for t in c["turns"]),
            )
            for c in runs[0]["conversations"]
            if c["status"] == "complete"
        ]
    result = []
    for key, group in candidate_sets(runs).items():
        if any(
            r["status"] != "complete"
            or any(c["status"] != "complete" for c in r["conversations"])
            for r in group
        ):
            continue
        text = "\n\n".join(
            f"Set {i + 1}, conversation {c['index'] + 1}\n"
            + "\n\n".join(f"{t['role']}: {t['text']}" for t in c["turns"])
            for i, run in enumerate(group)
            for c in run["conversations"]
        )
        result.append(dict(id=key, prompt="", text=text))
    return result


def selected_seed(project, runs, key):
    from .simulator import batch_seed, conversation_seed

    if len(runs) == 1 and "alternative_group" not in runs[0]:
        return conversation_seed(project, runs[0]["id"], int(key))
    group = candidate_sets(runs).get(str(key))
    if not group:
        raise ValueError("Selected alternative no longer exists")
    seeds = [batch_seed(project, run["id"]) for run in group]
    result = seeds[0] if len(seeds) == 1 else dict(sets=seeds)
    if group[0].get("alternative_scope"):
        result["scope"] = copy.deepcopy(group[0]["alternative_scope"])
    return result


def affected_targets(runs):
    return [
        dict(run=run["id"], conversation=c["index"])
        for run in runs
        for c in run["conversations"]
        if c["status"] == "complete"
        and c["index"]
        in run.get("affected_conversations", [v["index"] for v in run["conversations"]])
    ]


def alternative_id():
    return uuid.uuid4().hex[:12]


def scope_for_sets(sets):
    """Reconstruct a scope for old saved runs without changing their records."""
    groups = []
    for item in sets:
        children = [
            dict(kind="conversation", **leaf["parent"]) for leaf in members(item)
        ]
        groups.append(
            children[0]
            if "turns" in item
            else dict(kind="set", id=item["parent"]["run"], children=children)
        )
    return groups[0] if len(groups) == 1 else dict(kind="set", children=groups)


def seed_from_runs(project, runs):
    """Freeze precisely the affected leaves for a subsequent in-place loop."""
    scopes = []
    for run in runs:
        indices = run.get(
            "affected_conversations", [c["index"] for c in run["conversations"]]
        )
        scopes.append(
            dict(
                kind="set",
                id=run["id"],
                children=[
                    dict(kind="conversation", run=run["id"], conversation=i)
                    for i in indices
                ],
            )
        )
    return resolve_seed(
        project,
        dict(
            scope=scopes[0] if len(scopes) == 1 else dict(kind="set", children=scopes)
        ),
    )


def attach_scopes(runs, seed):
    """Store containment separately from ancestry; shared trees retain set boundaries."""
    source_scope = seed.get("scope") or scope_for_sets(seed_sets(seed))
    for group in candidate_sets(runs).values():
        mapping = {}
        for run in group:
            for conversation in run["conversations"]:
                parent = conversation.get("parent")
                if parent:
                    mapping[(parent["run"], parent["conversation"])] = dict(
                        kind="conversation",
                        run=run["id"],
                        conversation=conversation["index"],
                    )
        tree = action_scope.map_leaves(
            source_scope, lambda leaf: mapping[(leaf["run"], leaf["conversation"])]
        )

        def identify(node, path=""):
            if node["kind"] == "set":
                node["id"] = (
                    f"{group[0].get('alternative_group', group[0]['id'])}/{group[0].get('alternative_index', 0)}/s{path}"
                )
                for i, child in enumerate(node["children"]):
                    identify(child, path + f".{i}")

        identify(tree)
        for run in group:
            run["source_scope"] = copy.deepcopy(source_scope)
            run["alternative_scope"] = copy.deepcopy(tree)
