"""Resolve Simulator actions without UI state or inference side effects.

A run is a set of conversations. Alternative groups relate new runs without
flattening their members. Continued heads keep their IDs and archive the exact
previous run version before any streaming mutation.
"""

import copy
import uuid

from .domain import now


def seed_sets(seed):
    return seed.get("sets", [seed]) if seed else [None]


def members(seed):
    return seed.get("conversations", [seed]) if seed else []


def resolve_seed(project, args):
    from .simulator import batch_seed, conversation_seed

    if "targets" in args:
        targets = args["targets"]
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
        return sets[0] if len(sets) == 1 else {"sets": sets}
    if "run" not in args:
        return None
    return (
        conversation_seed(project, args["run"], args["conversation"])
        if "conversation" in args
        else batch_seed(project, args["run"], args.get("conversations"))
    )


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
    return seeds[0] if len(seeds) == 1 else dict(sets=seeds)


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
