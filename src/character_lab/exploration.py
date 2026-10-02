"""Repeated Loom batches. Generation stays in its document/conversation owner.

Selection classifies every candidate, then picks one eligible result. There is
no random fallback, confidence ranking or automatic anthology membership.
"""

import asyncio
import copy
import json
import uuid
from pathlib import Path

from . import assessments, templates
from .domain import now
from .policy import CHOICE_RESPONSE, DEFAULT_PROMPT, DEFAULT_SPEC, validate


def require_selector(model):
    if not Path(model.get("path", "")).is_file():
        raise ValueError("Configure a selection model in /policy before using --loops")


async def assess_candidate(project, candidate, run, judge, assessment):
    """Persist each attempted behavior call, including failures and cancellation."""
    judge_config = run["assessment_judge"]
    records = assessment["records"]
    records.extend(
        dict(
            text=candidate["text"],
            definition={
                **b,
                **judge_config,
                "scope": "whole",
                "expected": b.get("expected", "present"),
                "threshold": b.get("threshold", 0.8),
            },
            status="queued",
        )
        for b in run["behaviors"]
    )
    groups = (
        [records] if judge_config["call_mode"] == "bundled" else [[r] for r in records]
    )
    for group in groups:
        for record in group:
            record["status"] = "running"
        project.save()
        try:
            results = await assessments.assess_group(group, judge)
            for record, result in zip(group, results):
                record.update(status="complete", result=result)
        except asyncio.CancelledError:
            for record in group:
                record["status"] = "stopped"
            raise
        except Exception as exc:
            # A failed call is unknown, never evidence that a criterion failed.
            for record in group:
                record.update(status="failed", error=str(exc))
        finally:
            project.save()
    assessment["eligible"] = all(
        r["status"] == "complete" and r["result"]["passed"] for r in records
    )


async def explore(project, loops, model, runtime_factory, batch, advance, emit):
    """batch() returns immutable candidate views; advance() changes the next seed."""
    require_selector(model)
    behaviors = copy.deepcopy(project.data.get("selection_behaviors"))
    if behaviors is not None and not any(b.get("enabled", True) for b in behaviors):
        raise ValueError(
            "Enable at least one selection behavior before using selection"
        )
    if behaviors is None:
        behaviors = [
            dict(
                id="criteria",
                name="Selection criteria",
                spec=project.data.get("policy_spec", DEFAULT_SPEC),
                enabled=True,
            )
        ]
    behaviors = [b for b in behaviors if b.get("enabled", True)]
    assessment_judge = dict(
        kind="llm",
        model=model.get("alias") or model.get("name", ""),
        prompt=project.data.get(
            "selection_assessment_prompt", assessments.DEFAULT_PROMPT
        ),
        call_mode=project.data.get("selection_call_mode", "separate"),
    )
    run = dict(
        id=uuid.uuid4().hex[:12],
        created=now(),
        status="running",
        loops=loops,
        steps=[],
        selected=None,
        policy_model=copy.deepcopy(model),
        behaviors=behaviors,
        assessment_judge=assessment_judge,
        spec=project.data.get("policy_spec", DEFAULT_SPEC),
        prompt=project.data.get("policy_prompt", DEFAULT_PROMPT),
    )
    key = project.data.get("active_operational_policies", {}).get("selection")
    named = next(
        (
            p
            for p in project.data.get("operational_policies", {}).get("selection", [])
            if p["id"] == key
        ),
        None,
    )
    if named:
        run["policy"] = copy.deepcopy(named)
    if not run["spec"].strip() or not run["prompt"].strip():
        raise ValueError("Selection criteria and prompt are required in /policy")
    templates.validate_assessment(assessment_judge["prompt"])
    templates.validate_choice(run["prompt"])
    project.data.setdefault("policy_runs", []).append(run)
    project.save()
    judge = runtime_factory(project.folder, run["policy_model"])
    try:
        for index in range(loops):
            step = dict(
                loop=index + 1, parent=run["selected"], status="generating", trace={}
            )
            run["steps"].append(step)
            project.save()
            candidates = await batch(run["id"], index)
            if not candidates:
                raise ValueError(
                    "No completed candidates to select; partial outputs are retained"
                )
            candidates = copy.deepcopy(candidates)
            step["candidates"] = [c["id"] for c in candidates]
            step["inputs"] = candidates
            step["assessments"] = []
            eligible = []
            step["status"] = "assessing"
            await emit(
                "operation", dict(stage="assessing", loop=index + 1, loops=loops)
            )
            for candidate in candidates:
                assessment = dict(candidate=candidate["id"], records=[])
                step["assessments"].append(assessment)
                await assess_candidate(project, candidate, run, judge, assessment)
                if assessment["eligible"]:
                    eligible.append(candidate)
            step["eligible"] = [c["id"] for c in eligible]
            if not eligible:
                errors = any(
                    r["status"] == "failed"
                    for a in step["assessments"]
                    for r in a["records"]
                )
                if errors:
                    raise ValueError(
                        "No eligible candidate; some behavior assessments failed"
                    )
                step.update(
                    status="complete",
                    decision=dict(
                        selected=None,
                        reviews=[],
                        reason="No candidate met every enabled behavior",
                    ),
                )
                run.update(status="no_selection", selected=None)
                break
            step["status"] = "selecting"
            state = dict(
                spec=run["spec"],
                behaviors=copy.deepcopy(run["behaviors"]),
                candidates=[
                    dict(
                        node=c["id"],
                        parent=c["prompt"],
                        continuation=c["text"][len(c["prompt"]) :],
                    )
                    for c in eligible
                ],
                assessments=[
                    dict(
                        candidate=a["candidate"],
                        results=[
                            dict(behavior=r["definition"]["id"], result=r["result"])
                            for r in a["records"]
                        ],
                    )
                    for a in step["assessments"]
                    if a["eligible"]
                ],
            )
            context = {
                **state,
                "behaviors": templates.behavior_objects(run["behaviors"]),
            }
            if templates.variables(run["prompt"]):
                step["trace"]["template_context"] = context
                step["messages"] = [
                    dict(
                        role="system",
                        content=CHOICE_RESPONSE,
                    ),
                    dict(role="user", content=templates.render(run["prompt"], context)),
                ]
            else:
                step["messages"] = [
                    dict(role="system", content=run["prompt"]),
                    dict(role="user", content=json.dumps(state, ensure_ascii=False)),
                ]
            project.save()
            await emit(
                "operation", dict(stage="selecting", loop=index + 1, loops=loops)
            )
            result = await judge.judge(step["messages"], step["trace"])
            step["raw_decision"] = result
            step["decision"] = validate(result, eligible)
            step["status"] = "complete"
            run["selected"] = result["selected"]
            project.save()
            judge.close()
            if result["selected"] is None:
                run["status"] = "no_selection"
                break
            await advance(result["selected"])
        else:
            run["status"] = "complete"
    except BaseException as exc:
        run.update(
            status="stopped" if isinstance(exc, asyncio.CancelledError) else "failed",
            error=str(exc),
        )
        if run["steps"]:
            run["steps"][-1]["status"] = run["status"]
        if isinstance(exc, asyncio.CancelledError):
            for step in run["steps"]:
                for assessment in step.get("assessments", []):
                    for record in assessment["records"]:
                        if record["status"] in {"queued", "running"}:
                            record["status"] = "stopped"
        raise
    finally:
        judge.close()
        run["finished"] = now()
        project.save()
    await emit(
        "operation", dict(stage=run["status"], message="Loom loops: " + run["status"])
    )
    return run
