"""Named evaluation datasets: membership and evidence, separate from judging.

Items freeze source revisions. Judgments remain immutable records in evaluations;
collections reference them rather than copying or rerunning existing results.
"""

import asyncio
import copy
import hashlib
import json
import uuid

from . import evaluation, evaluation_policies
from .domain import now
from .exploration import require_selector


def uid():
    return uuid.uuid4().hex[:12]


def collections(project):
    return project.data.get("evaluation_sets", [])


def resolve(project, name=None):
    key = name or project.data.get("active_evaluation", "")
    matches = [s for s in collections(project) if s["id"] == key or s["name"] == key]
    if len(matches) != 1:
        raise ValueError("Choose a data collection in Evaluate")
    return matches[0]


def migrate(project):
    """One-time, lossless membership migration; original records stay untouched."""
    if "evaluation_sets" in project.data:
        evaluation_policies.migrate(project)
        return
    project.data["evaluation_sets"] = []
    definitions = {d["id"]: d for d in evaluation.definitions(project)}
    for record in project.data.get("evaluations", []):
        definitions.setdefault(record["definition"]["id"], record["definition"])
    used = set()
    for key, definition in definitions.items():
        name = definition["name"]
        if name in used:
            name += " · " + key
        used.add(name)
        group = dict(id=uid(), name=name, judges=[key], items=[], created=now())
        project.data["evaluation_sets"].append(group)
        for record in project.data.get("evaluations", []):
            if record["definition"]["id"] == key:
                item = add_capture(
                    group, evaluation.capture(project, {"evaluation": record["id"]})
                )
                item["judgments"].append(record["id"])
                item["training"] |= record.get("training", False)
                if record.get("note"):
                    item["note"] = record["note"]
    if collections(project):
        project.data["active_evaluation"] = collections(project)[0]["id"]
    evaluation_policies.migrate(project)
    project.save()


def add_capture(group, captured):
    digest = hashlib.sha256(
        json.dumps(captured, sort_keys=True, ensure_ascii=False).encode()
    ).hexdigest()
    for item in group["items"]:
        if item["snapshot_hash"] == digest:
            return item
    item = dict(
        **copy.deepcopy(captured),
        id=uid(),
        snapshot_hash=digest,
        judgments=[],
        evidence=[],
        training=False,
        note="",
        created=now(),
        metadata_history=[],
    )
    group["items"].append(item)
    return item


def records_for(project, item):
    known = {r["id"]: r for r in project.data.get("evaluations", [])}
    return [known[key] for key in item["judgments"] if key in known]


def item_summary(project, group, item):
    records = records_for(project, item)
    # Aggregate the latest result of every currently configured judge revision.
    latest = {}
    for record in records:
        definition = record["definition"]
        latest[(definition["id"], definition["revision"])] = record
    definitions = {d["id"]: d for d in evaluation.definitions(project)}
    policy = evaluation_policies.active(project)
    judge_ids = policy["judges"] if policy else group.get("judges", [])
    relevant = [
        latest.get((key, definitions[key]["revision"])) if key in definitions else None
        for key in judge_ids
    ]
    status, passed = "unjudged", None
    if relevant and all(r and r["status"] == "complete" for r in relevant):
        status, passed = "complete", all(r["passed"] for r in relevant)
    elif any(r and r["status"] in {"queued", "running"} for r in relevant):
        status = "running"
    elif any(
        r and r["status"] in {"failed", "stopped", "interrupted"} for r in relevant
    ):
        status = "incomplete"
    elif records or item["evidence"]:
        status = "evidence"
    return {k: item[k] for k in ("id", "title", "kind", "training", "created")} | dict(
        status=status,
        passed=passed,
        judgments=len(records),
        evidence=len(item["evidence"]),
    )


def summaries(project):
    return [
        {k: s[k] for k in ("id", "name")}
        | {"items": [item_summary(project, s, i) for i in s["items"]]}
        for s in collections(project)
    ]


def data_collection(project, key=None):
    if key:
        return resolve(project, key)
    if collections(project):
        current = project.data.get("active_evaluation")
        if not any(g["id"] == current for g in collections(project)):
            project.data["active_evaluation"] = collections(project)[0]["id"]
        return resolve(project)
    group = dict(id=uid(), name="Data", items=[], created=now())
    project.data.setdefault("evaluation_sets", []).append(group)
    project.data["active_evaluation"] = group["id"]
    return group


def plan(session, name=None, collection=None):
    policy = evaluation_policies.resolve(session.project, name)
    definitions = evaluation_policies.Definitions(
        policy,
        [
            copy.deepcopy(evaluation.find(evaluation.definitions(session.project), key))
            for key in policy["judges"]
        ],
    )
    if not definitions:
        raise ValueError("Add at least one behavior to this policy first")
    for definition in definitions:
        if definition["kind"] == "llm":
            require_selector(session.policy_model)
            if definition["model"] != session.policy_model.get("alias"):
                raise ValueError(
                    "Choose the configured local policy model for this behavior"
                )
    return data_collection(session.project, collection), definitions


def prepare(session, group, definitions, items):
    if not items or any(not i["text"].strip() for i in items):
        raise ValueError("Select nonempty saved items to evaluate")
    batch = uid()
    records = []
    for item in items:
        for definition in definitions:
            record = {
                k: copy.deepcopy(item[k])
                for k in (
                    "target",
                    "kind",
                    "title",
                    "text",
                    "source",
                    "generation_models",
                )
            }
            record.update(
                id=uid(),
                batch=batch,
                created=now(),
                status="queued",
                definition=copy.deepcopy(definition),
                content_hash=hashlib.sha256(item["text"].encode()).hexdigest(),
                trace={},
                training=False,
                train_on_pass=False,
                note="",
                metadata_history=[],
                collection=group["id"],
                item=item["id"],
            )
            records.append(record)
            item["judgments"].append(record["id"])
    evaluation_policies.record_run(
        session.project, batch, group, definitions, items, records
    )
    session.project.data.setdefault("evaluations", []).extend(records)
    session.project.save()
    return records


async def run(session, group, definitions, items, auto=False, *, nested=False):
    records = prepare(session, group, definitions, items)
    session.runtime.close()
    try:
        await evaluation.evaluate(session, records, manage_job=False)
        if auto:
            for item in items:
                own = [r for r in records if r["item"] == item["id"]]
                if all(
                    r.get("passed") is True and r["status"] == "complete" for r in own
                ):
                    item["training"] = True
                    item.setdefault("metadata_history", []).append(
                        dict(at=now(), training=True, origin="train_on_pass")
                    )
            session.project.save()
    finally:
        if not nested:
            session.job = None
        await session.snapshot()


def attach_policy_evidence(project, item):
    """Attach observed evidence with exact scope; never invent a whole-item grade."""
    evidence = []
    if item["kind"] == "conversation":
        source = item["source"]
        for n, turn in enumerate(source["conversation"]["turns"]):
            for check in [*turn.get("monitor_checks", []), turn.get("monitor", {})]:
                if check.get("status") == "complete":
                    evidence.append(
                        dict(
                            origin="monitoring",
                            scope=f"turn {n + 1}",
                            result=copy.deepcopy(check),
                        )
                    )
    else:
        source = item["source"]
        for check in [*source.get("monitor_checks", []), source.get("monitor", {})]:
            if check.get("status") == "complete":
                evidence.append(
                    dict(
                        origin="monitoring",
                        scope="document generation",
                        result=copy.deepcopy(check),
                    )
                )
    # Selector evidence includes all candidate context; its outcome is relative.
    for run in project.data.get("policy_runs", []):
        for step in run.get("steps", []):
            if step.get("status") == "complete" and (
                (
                    item["kind"] == "document"
                    and (
                        item["target"].get("node") in step.get("candidates", [])
                        or item["source"].get("document_set")
                        in step.get("candidates", [])
                    )
                )
                or (
                    item["kind"] == "conversation"
                    and run["id"] == item["source"].get("policy_run")
                    and step["loop"] == item["source"].get("loop")
                    and str(
                        item["source"].get(
                            "alternative_index", item["target"]["conversation"]
                        )
                    )
                    in step.get("candidates", [])
                )
            ):
                evidence.append(
                    dict(
                        origin="selection",
                        scope="candidate set",
                        result={
                            "step": copy.deepcopy(step),
                            "spec": run["spec"],
                            "prompt": run["prompt"],
                            "model": run["policy_model"],
                        },
                    )
                )
    for record in evidence:
        if record not in item["evidence"]:
            item["evidence"].append(record)


async def dispatch(session, command, args, request_id):
    p = session.project
    if command == "evaluation.collection.save":
        name = args.get("name", "").strip()
        if not name:
            raise ValueError("Name the data collection")
        old = resolve(p, args["id"]) if args.get("id") else None
        if any(s["name"] == name and s is not old for s in collections(p)):
            raise ValueError("A data collection already has that name")
        judges = list(dict.fromkeys(args.get("judges", [])))
        for key in judges:
            evaluation.find(evaluation.definitions(p), key)
        if old:
            old.update(name=name)
        else:
            old = dict(id=uid(), name=name, items=[], created=now())
            p.data.setdefault("evaluation_sets", []).append(old)
            p.data["active_evaluation"] = old["id"]
        # Older clients supplied judges here; move that configuration into a policy.
        if "judges" in args:
            policy = evaluation_policies.save(
                p, {"id": old.get("legacy_policy"), "name": name, "judges": judges}
            )
            old["legacy_policy"] = policy["id"]
    elif command == "evaluation.collection.active":
        p.data["active_evaluation"] = resolve(p, args["collection"])["id"]
    else:
        if command == "evaluation.collection.run":
            group, definitions = plan(
                session, args.get("policy"), args.get("collection")
            )
        else:
            group = data_collection(p, args.get("collection"))
        if command == "evaluation.item.open":
            item = evaluation.find(group["items"], args["id"])
            result = copy.deepcopy(item) | item_summary(p, group, item)
            result["judgments"] = copy.deepcopy(records_for(p, item))
            result["evidence"] = copy.deepcopy(item["evidence"])
            await session.emit("evaluation", result, request_id)
            return
        if command == "evaluation.collection.add":
            captures = [evaluation.capture(p, t) for t in args.get("targets", [])]
            for target, captured in zip(args.get("targets", []), captures):
                item = add_capture(group, captured)
                if (
                    "evaluation" in target
                    and target["evaluation"] not in item["judgments"]
                ):
                    item["judgments"].append(target["evaluation"])
                if args.get("attach_evidence"):
                    attach_policy_evidence(p, item)
        elif command == "evaluation.collection.remove":
            ids = args.get("ids")
            if (
                not isinstance(ids, list)
                or not ids
                or any(not isinstance(key, str) for key in ids)
            ):
                raise ValueError("Select evaluation items to remove")
            items = [evaluation.find(group["items"], key) for key in dict.fromkeys(ids)]
            # Removal changes membership, never destroys judgments or frozen
            # research evidence. Keep a recoverable record outside active items.
            for item in items:
                group.setdefault("removed_items", []).append(
                    {"removed": now(), "item": copy.deepcopy(item)}
                )
            group["items"] = [i for i in group["items"] if i["id"] not in ids]
        elif command == "evaluation.collection.run":
            auto = args.get("train_on_pass", False)
            if type(auto) is not bool:
                raise ValueError("train_on_pass must be boolean")
            captured = [evaluation.capture(p, t) for t in args.get("targets", [])]
            items = [
                evaluation.find(group["items"], key) for key in args.get("items", [])
            ]
            items += [add_capture(group, c) for c in captured]
            items = list({i["id"]: i for i in items}.values())
            if not items:
                raise ValueError(
                    "Select items to run; adding and running are separate actions"
                )
            if any(not i["text"].strip() for i in items):
                raise ValueError("Cannot evaluate empty text")
            p.save()
            session.job_id = request_id
            session.job = asyncio.create_task(
                run(session, group, definitions, items, auto)
            )
        elif command == "evaluation.item.annotate":
            items = [evaluation.find(group["items"], key) for key in args["ids"]]
            for key, expected in (("training", bool), ("note", str)):
                if key in args and type(args[key]) is not expected:
                    raise ValueError("Invalid item metadata")
            for item in items:
                changes = {k: args[k] for k in ("training", "note") if k in args}
                item.update(changes)
                item.setdefault("metadata_history", []).append(
                    dict(at=now(), origin="human", **changes)
                )
        elif command == "evaluation.collection.export":
            folder = p.folder / "datasets"
            folder.mkdir(exist_ok=True)
            path = folder / f"evaluation-{group['id']}-{uid()}.jsonl"
            records = [
                copy.deepcopy(i)
                | {
                    "judgments": records_for(p, i),
                    "collection": {"id": group["id"], "name": group["name"]},
                }
                for i in group["items"]
                if i["training"]
            ]
            if not records:
                raise ValueError("Mark items for training first")
            temporary = path.with_suffix(".tmp")
            temporary.write_text(
                "".join(json.dumps(r, ensure_ascii=False) + "\n" for r in records)
            )
            temporary.replace(path)
            await session.emit("result", {"path": str(path)}, request_id)
        else:
            raise ValueError("Unknown evaluation collection command")
    p.save()
    await session.snapshot(request_id)


async def after_generation(session, eval_plan, targets):
    if not targets:
        return
    group, definitions = eval_plan
    items = [
        add_capture(group, evaluation.capture(session.project, t)) for t in targets
    ]
    session.project.save()
    await run(session, group, definitions, items, nested=True)
