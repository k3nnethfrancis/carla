"""Reusable assessment policies and frozen run envelopes.

Data membership lives in evaluation_sets. Policies own judge configurations and a shared set of behaviors;
run envelopes freeze that configuration and reference immutable assessment records.
"""

import copy
import uuid

from . import evaluation, evaluation_judges
from .domain import now


def uid():
    return uuid.uuid4().hex[:12]


def active(project):
    key = project.data.get("active_evaluation_policy")
    return next(
        (p for p in project.data.get("evaluation_policies", []) if p["id"] == key), None
    )


def resolve(project, key=None):
    key = key or project.data.get("active_evaluation_policy")
    matches = [
        p
        for p in project.data.get("evaluation_policies", [])
        if key in (p["id"], p["name"])
    ]
    if len(matches) != 1:
        raise ValueError(
            "Choose a policy in Evaluate → Policies, or use /eval policy-name"
        )
    return matches[0]


class Definitions(list):
    """Carry the frozen policy alongside the existing generation-plan sequence."""

    def __init__(self, policy, definitions):
        super().__init__(definitions)
        self.policy = copy.deepcopy(policy)


def save(project, args):
    policies = project.data.setdefault("evaluation_policies", [])
    old = resolve(project, args["id"]) if args.get("id") else None
    name = args.get("name", "").strip()
    if not name:
        raise ValueError("Name the policy")
    if any(p["name"] == name and p is not old for p in policies):
        raise ValueError("A policy already has that name")
    judges = args.get("judges", [])
    # Old clients sent evaluator IDs. Convert immediately to policy-owned objects.
    if isinstance(judges, list):
        judges = [
            evaluation_judges.legacy(
                evaluation.find(evaluation.definitions(project), j)
            )
            if isinstance(j, str)
            else j
            for j in judges
        ]
    proposed = {"judges": copy.deepcopy(judges)}
    if "behaviors" in args:
        proposed["behaviors"] = copy.deepcopy(args["behaviors"])
    evaluation_judges.lift_behaviors(proposed)
    judges = evaluation_judges.normalize(
        proposed["judges"], old["judges"] if old else []
    )
    behaviors = evaluation_judges.normalize_behaviors(
        proposed["behaviors"], old.get("behaviors", []) if old else []
    )
    actions = args.get(
        "actions",
        old.get("actions", {"train_on_pass": False})
        if old
        else {"train_on_pass": False},
    )
    if (
        not isinstance(actions, dict)
        or set(actions) != {"train_on_pass"}
        or type(actions["train_on_pass"]) is not bool
    ):
        raise ValueError("Evaluation actions require boolean train_on_pass")
    if old:
        old.update(
            name=name,
            judges=judges,
            behaviors=behaviors,
            actions=copy.deepcopy(actions),
            revision=old["revision"] + 1,
        )
    else:
        old = dict(
            id=uid(),
            name=name,
            judges=judges,
            behaviors=behaviors,
            actions=copy.deepcopy(actions),
            revision=1,
        )
        policies.append(old)
    if not active(project):
        project.data["active_evaluation_policy"] = old["id"]
    return old


def migrate(project):
    """Split legacy configuration without changing existing data or judgments."""
    changed = False
    if "evaluation_policies" not in project.data:
        project.data["evaluation_policies"] = []
        for group in project.data.get("evaluation_sets", []):
            if not group.get("judges"):
                continue
            policy = dict(
                id=uid(), name=group["name"], judges=list(group["judges"]), revision=1
            )
            project.data["evaluation_policies"].append(policy)
            group["legacy_policy"] = policy["id"]
            if group["id"] == project.data.get("active_evaluation"):
                project.data["active_evaluation_policy"] = policy["id"]
        if not active(project) and project.data["evaluation_policies"]:
            project.data["active_evaluation_policy"] = project.data[
                "evaluation_policies"
            ][0]["id"]
        changed = True
    definitions = {d["id"]: d for d in evaluation.definitions(project)}
    for policy in project.data["evaluation_policies"]:
        if any(isinstance(j, str) for j in policy["judges"]):
            policy["judges"] = [
                evaluation_judges.legacy(definitions[j])
                if isinstance(j, str) and j in definitions
                else {
                    "id": j,
                    "name": "Missing legacy judge",
                    "missing": True,
                    "behaviors": [],
                }
                if isinstance(j, str)
                else j
                for j in policy["judges"]
            ]
            changed = True
        changed = evaluation_judges.lift_behaviors(policy) or changed
    if "evaluation_runs" not in project.data:
        project.data["evaluation_runs"] = []
        # Legacy standalone judgments may never have belonged to a collection.
        # Recover their exact frozen captures into Data without rewriting records.
        from .evaluation_sets import add_capture

        linked = {
            key
            for group in project.data.get("evaluation_sets", [])
            for item in group["items"]
            for key in item["judgments"]
        }
        unlinked = [
            r for r in project.data.get("evaluations", []) if r["id"] not in linked
        ]
        if unlinked:
            group = dict(id=uid(), name="Historical data", items=[], created=now())
            project.data.setdefault("evaluation_sets", []).append(group)
            if not any(
                g["id"] == project.data.get("active_evaluation")
                for g in project.data["evaluation_sets"]
            ):
                project.data["active_evaluation"] = group["id"]
            for record in unlinked:
                item = add_capture(
                    group, evaluation.capture(project, {"evaluation": record["id"]})
                )
                item["judgments"].append(record["id"])
                item["training"] |= record.get("training", False)
                if record.get("note"):
                    item["note"] = record["note"]
        batches = {}
        for record in project.data.get("evaluations", []):
            batches.setdefault(record.get("batch") or record["id"], []).append(record)
        for batch, records in batches.items():
            definitions = []
            for record in records:
                if record["definition"] not in definitions:
                    definitions.append(copy.deepcopy(record["definition"]))
            project.data["evaluation_runs"].append(
                dict(
                    id=batch,
                    created=records[0]["created"],
                    policy=dict(name="Historical assessment", judges=definitions),
                    collection=records[0].get("collection", ""),
                    items=list(
                        dict.fromkeys(r["item"] for r in records if r.get("item"))
                    ),
                    records=[r["id"] for r in records],
                )
            )
        changed = True
    if changed:
        project.save()


def record_run(project, batch, group, definitions, items, records):
    project.data.setdefault("evaluation_runs", []).append(
        dict(
            id=batch,
            created=now(),
            policy=copy.deepcopy(definitions.policy),
            collection=group["id"],
            collection_name=group["name"],
            items=[i["id"] for i in items],
            records=[r["id"] for r in records],
        )
    )


def run_summary(project, run):
    known = {r["id"]: r for r in project.data.get("evaluations", [])}
    records = [known[k] for k in run["records"] if k in known]
    states = {r["status"] for r in records}
    complete = len(records) == len(run["records"]) and states == {"complete"}
    status = (
        "complete"
        if complete
        else ("running" if states & {"queued", "running"} else "incomplete")
    )
    return copy.deepcopy(run) | dict(
        status=status,
        passed=all(r.get("passed") is True for r in records) if complete else None,
        count=len(records),
        completed=sum(r["status"] == "complete" for r in records),
    )


def run_summaries(project):
    return [run_summary(project, r) for r in project.data.get("evaluation_runs", [])]


def run_view(project, run):
    result = run_summary(project, run)
    result["results"] = [
        copy.deepcopy(evaluation.find(project.data.get("evaluations", []), k))
        for k in run["records"]
    ]
    active = next((r for r in result["results"] if r["status"] == "running"), None)
    if active:
        result["progress"] = dict(
            run=run["id"],
            record=active["id"],
            title=active["title"],
            judge=active["definition"]["name"],
            stage="Assessing",
            text=active.get("trace", {}).get("raw_response", ""),
        )
    return result


async def publish_run(session, batch, *, opened=False):
    if not batch:
        return
    run = evaluation.find(session.project.data.get("evaluation_runs", []), batch)
    await session.emit(
        "evaluation_run",
        run_view(session.project, run) | {"opened": opened, "live": True},
        session.job_id,
    )


async def dispatch(session, command, args, request_id):
    p = session.project
    if command == "evaluation.policy.save":
        save(p, args)
    elif command == "evaluation.policy.active":
        p.data["active_evaluation_policy"] = resolve(p, args["policy"])["id"]
    elif command == "evaluation.policy.delete":
        policy = resolve(p, args["id"])
        p.data["evaluation_policies"].remove(policy)
        if p.data.get("active_evaluation_policy") == policy["id"]:
            remaining = p.data["evaluation_policies"]
            p.data["active_evaluation_policy"] = remaining[0]["id"] if remaining else ""
    elif command == "evaluation.run.open":
        run = evaluation.find(p.data.get("evaluation_runs", []), args["id"])
        result = run_view(p, run)
        await session.emit("evaluation_run", result, request_id)
        return
    else:
        raise ValueError("Unknown evaluation policy/run command")
    p.save()
    await session.snapshot(request_id)
