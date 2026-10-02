"""Whole-item evaluations and training selections, independent of rendering.

Results freeze input and judge configuration. Notes/training membership are mutable
metadata with an audit trail; reruns append results rather than rewriting evidence.
"""

import asyncio
import copy
import hashlib
import json
import uuid
from pathlib import Path

from . import assessments
from .domain import display_title, now
from .monitor import LOCAL_URL, classify, local_url

DEFAULT_PROMPT = assessments.DEFAULT_PROMPT


def resolve_model(session, alias):
    """Preflight local judge weights before generation or evaluation queues work."""
    model = session.judge_model(alias)
    if not Path(model.get("path", "")).is_file():
        raise ValueError(
            f"Evaluation judge {alias!r} model file is unavailable; configure it in /policy"
        )
    return copy.deepcopy(model)


def definitions(project):
    return project.data.get("evaluators", [])


def find(items, key):
    item = next((item for item in items if item["id"] == key), None)
    if item is None:
        raise ValueError("Saved evaluation or definition was not found")
    return item


def summaries(project):
    return [
        {
            k: r.get(k)
            for k in (
                "id",
                "title",
                "kind",
                "status",
                "passed",
                "training",
                "created",
                "error",
            )
        }
        | {
            "evaluator": r["definition"]["name"],
            "revision": r["definition"]["revision"],
        }
        for r in project.data.get("evaluations", [])
    ]


def save_definition(project, args):
    items = project.data.setdefault("evaluators", [])
    old = find(items, args["id"]) if args.get("id") else None
    definition = {
        k: args.get(k) for k in ("name", "kind", "spec", "prompt", "model", "threshold")
    }
    for field in ("name", "spec"):
        if not isinstance(definition[field], str) or not definition[field].strip():
            raise ValueError(f"Evaluation needs {field}")
    if definition["kind"] not in {"llm", "jev", "diffusion"}:
        raise ValueError("Choose an LLM, Jev or DiffusionGemma judge")
    if definition["kind"] == "diffusion":
        definition["endpoint"] = local_url(args.get("endpoint", LOCAL_URL))
    if not isinstance(definition["prompt"], str):
        raise ValueError("Judge prompt must be text")
    if definition["kind"] == "llm" and not definition["prompt"].strip():
        raise ValueError("LLM judge needs a prompt")
    if not isinstance(definition["model"], str) or not definition["model"].strip():
        raise ValueError("Choose a judge model")
    threshold = definition["threshold"]
    if type(threshold) not in (float, int) or not 0 < threshold <= 1:
        raise ValueError(
            "Classifier pass threshold must be greater than 0 and at most 1"
        )
    definition["expected"] = assessments.expected_state(args)
    definition.update(
        id=old["id"] if old else uuid.uuid4().hex[:12],
        revision=old["revision"] + 1 if old else 1,
    )
    if old:
        items[items.index(old)] = definition
    else:
        items.append(definition)
    project.save()
    return definition


def capture(project, target):
    """Freeze full text plus generation provenance, never a truncated preview."""
    if "evaluation" in target:
        old = find(project.data.get("evaluations", []), target["evaluation"])
        return {
            k: copy.deepcopy(old[k])
            for k in ("target", "kind", "title", "text", "source")
        } | {"generation_models": copy.deepcopy(old.get("generation_models", []))}
    if "node" in target:
        node = next(
            (n for n in project.data["nodes"] if n["id"] == target["node"]), None
        )
        if node is None:
            raise ValueError("Document no longer exists")
        source = copy.deepcopy(node)
        source["ancestors"] = []
        parent = node.get("parent")
        while parent:
            ancestor = project.node(parent)
            source["ancestors"].append(copy.deepcopy(ancestor))
            parent = ancestor.get("parent")
        source["origins"] = project.origins(node["id"])
        return dict(
            target={"node": node["id"]},
            kind="document",
            title=display_title(node),
            text=node["text"],
            source=source,
            generation_models=list(
                dict.fromkeys(
                    n.get("trace", {}).get("model", {}).get("name")
                    for n in [node, *source["ancestors"]]
                    if n.get("trace", {}).get("model", {}).get("name")
                )
            ),
        )
    if "run" in target and type(target.get("conversation")) is int:
        run = find(project.data.get("simulation_runs", []), target["run"])
        index = target["conversation"]
        if not 0 <= index < len(run["conversations"]):
            raise ValueError("Conversation not found")
        conversation = run["conversations"][index]
        text = "\n\n".join(f"{t['role']}:\n{t['text']}" for t in conversation["turns"])
        # Run metadata carries frozen anthology/settings; exclude sibling traces.
        source = {
            k: copy.deepcopy(v)
            for k, v in run.items()
            if k not in {"conversations", "revisions"}
        }
        source["conversation"] = copy.deepcopy(conversation)
        return dict(
            target={"run": run["id"], "conversation": index},
            kind="conversation",
            title=f"Conversation {index + 1} · {run['id']}",
            text=text,
            source=source,
            generation_models=list(
                dict.fromkeys(
                    f"{t['role']}: {t['model']['name']}"
                    for t in conversation["turns"]
                    if t.get("model", {}).get("name")
                )
            ),
        )
    raise ValueError("Select a saved document or conversation to evaluate")


validate_result = assessments.validate_result


def interrupt_pending(project):
    """Also handles cancellation before the evaluation coroutine starts."""
    changed = False
    for record in project.data.get("evaluations", []):
        if record["status"] in {"queued", "running"}:
            record.update(status="stopped", finished=now())
            changed = True
    if changed:
        project.save()


async def assess_group(records, judge):
    """Compatibility boundary for callers that supply a classifier transport."""
    return await assessments.assess_group(records, judge, classify_fn=classify)


async def evaluate(session, records, *, manage_job=True):
    project = session.project
    judge = None
    judge_model = None
    groups = []
    bundled = {}
    for record in records:
        definition = record["definition"]
        if definition.get("call_mode") == "bundled":
            key = (record.get("item", record["id"]), definition["judge_id"])
            if key not in bundled:
                bundled[key] = []
                groups.append(bundled[key])
            bundled[key].append(record)
        else:
            groups.append([record])
    try:
        for index, group in enumerate(groups):
            definition = group[0]["definition"]
            model = None
            if definition["kind"] == "llm":
                model = definition.get("resolved_model") or session.judge_model(
                    definition["model"]
                )
            # Switching judges releases the previous managed model before starting
            # the next one. Frozen model configs keep a queued run reproducible.
            if judge is not None and model != judge_model:
                judge.close()
                judge = None
            if model is not None and judge is None:
                judge = session.runtime_factory(project.folder, model)
                judge_model = model
            for record in group:
                record["status"] = "running"
            project.save()
            await session.snapshot()
            await session.emit(
                "operation",
                {
                    "stage": "evaluating",
                    "message": f"Evaluating call {index + 1}/{len(groups)} · {group[0]['title']}",
                },
                session.job_id,
            )
            try:
                results = await assess_group(group, judge)
                for record, result in zip(group, results):
                    record.update(
                        status="complete", result=result, passed=result["passed"]
                    )
                    if record["train_on_pass"] and result["passed"]:
                        record["training"] = True
                        record["metadata_history"].append(
                            dict(at=now(), training=True, origin="train_on_pass")
                        )
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                for record in group:
                    record.update(status="failed", error=str(exc))
            finally:
                for record in group:
                    record["finished"] = now()
                    record["trace"] = copy.deepcopy(record["trace"])
                project.save()
            await session.snapshot()
    except asyncio.CancelledError:
        for record in records:
            if record["status"] in {"queued", "running"}:
                record.update(status="stopped", finished=now())
    except Exception as exc:
        for record in records:
            if record["status"] in {"queued", "running"}:
                record.update(status="failed", error=str(exc), finished=now())
    finally:
        if judge:
            judge.close()
        project.save()
        if manage_job:
            session.job = None
        await session.snapshot()
        passed = sum(r.get("passed") is True for r in records)
        failed = sum(r.get("passed") is False for r in records)
        other = len(records) - passed - failed
        await session.emit(
            "operation",
            {
                "stage": "complete",
                "message": f"Evaluation finished · {passed} pass · {failed} fail · {other} incomplete",
            },
            session.job_id,
        )


async def dispatch(session, command, args, request_id):
    p = session.project
    if command.startswith(("evaluation.policy.", "evaluation.run.")):
        from .evaluation_policies import dispatch as dispatch_policy

        await dispatch_policy(session, command, args, request_id)
        return
    if command.startswith(("evaluation.collection.", "evaluation.item.")):
        from .evaluation_sets import dispatch as dispatch_collection

        await dispatch_collection(session, command, args, request_id)
        return
    if command == "evaluation.configure":
        save_definition(p, args)
    elif command == "evaluation.definition.delete":
        items = p.data.setdefault("evaluators", [])
        if any(
            any(b["id"] == args["id"] for b in policy.get("behaviors", []))
            for policy in p.data.get("evaluation_policies", [])
        ):
            raise ValueError(
                "Remove this behavior from its policies before deleting it"
            )
        items.remove(find(items, args["id"]))
        p.save()
    elif command == "evaluation.open":
        record = find(p.data.get("evaluations", []), args["id"])
        await session.emit("evaluation", record, request_id)
        return
    elif command == "evaluation.annotate":
        ids = args.get("ids", [])
        records = [find(p.data.get("evaluations", []), key) for key in ids]
        if "training" in args and type(args["training"]) is not bool:
            raise ValueError("Training selection must be true or false")
        if "note" in args and not isinstance(args["note"], str):
            raise ValueError("Note must be text")
        if args.get("training") and any(r["status"] != "complete" for r in records):
            raise ValueError("Finish evaluation before marking an item for training")
        for record in records:
            changes = {k: args[k] for k in ("training", "note") if k in args}
            record.update(changes)
            record["metadata_history"].append(
                {"at": now(), "origin": "human", **changes}
            )
        p.save()
    elif command == "evaluation.export":
        records = [r for r in p.data.get("evaluations", []) if r.get("training")]
        if not records:
            raise ValueError("Mark evaluated items for training first")
        folder = p.folder / "datasets"
        folder.mkdir(exist_ok=True)
        path = folder / f"evaluated-{uuid.uuid4().hex[:12]}.jsonl"
        temporary = path.with_suffix(".tmp")
        with temporary.open("x", encoding="utf-8") as stream:
            for record in records:
                stream.write(json.dumps(record, ensure_ascii=False) + "\n")
        temporary.replace(path)
        await session.emit("result", {"path": str(path)}, request_id)
    elif command == "evaluation.run":
        definition = copy.deepcopy(find(definitions(p), args["definition"]))
        auto = args.get("train_on_pass", False)
        if type(auto) is not bool:
            raise ValueError("train_on_pass must be true or false")
        targets = args.get("targets")
        if not isinstance(targets, list) or not targets:
            raise ValueError("Select at least one item to evaluate")
        captures = [capture(p, target) for target in targets]
        if any(not item["text"].strip() for item in captures):
            raise ValueError("Cannot evaluate empty text")
        if definition["kind"] == "llm":
            definition["resolved_model"] = resolve_model(session, definition["model"])
        records = []
        batch_id = uuid.uuid4().hex[:12]
        for item in captures:
            records.append(
                dict(
                    **item,
                    id=uuid.uuid4().hex[:12],
                    batch=batch_id,
                    created=now(),
                    status="queued",
                    definition=copy.deepcopy(definition),
                    content_hash=hashlib.sha256(item["text"].encode()).hexdigest(),
                    trace={},
                    training=False,
                    train_on_pass=auto,
                    note="",
                    metadata_history=[],
                )
            )
        # Keep the original single-behavior endpoint visible in Data and Runs.
        from . import evaluation_policies, evaluation_sets

        group = evaluation_sets.data_collection(p)
        items = [evaluation_sets.add_capture(group, item) for item in captures]
        for item, record in zip(items, records):
            record.update(collection=group["id"], item=item["id"])
            item["judgments"].append(record["id"])
        frozen = evaluation_policies.Definitions(
            {"name": definition["name"]}, [definition]
        )
        evaluation_policies.record_run(p, batch_id, group, frozen, items, records)
        session.runtime.close()
        p.data.setdefault("evaluations", []).extend(records)
        p.save()
        session.job_id = request_id
        session.job = asyncio.create_task(evaluate(session, records))
    else:
        raise ValueError("Unknown evaluation command")
    await session.snapshot(request_id)
