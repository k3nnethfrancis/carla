"""Repeated Loom batches. Generation stays in its document/conversation owner.

Selection classifies every candidate, then picks one eligible result. There is
no random fallback, confidence ranking or automatic anthology membership.
"""

import asyncio
import json
import uuid
from pathlib import Path

from .domain import now
from .policy import DEFAULT_PROMPT, DEFAULT_SPEC, validate


def require_selector(model):
    if not Path(model.get("path", "")).is_file():
        raise ValueError("Configure a selection model in /policy before using --loops")


async def explore(project, loops, model, runtime_factory, batch, advance, emit):
    """batch() returns immutable candidate views; advance() changes the next seed."""
    require_selector(model)
    run = dict(
        id=uuid.uuid4().hex[:12],
        created=now(),
        status="running",
        loops=loops,
        steps=[],
        selected=None,
        policy_model=model.copy(),
        spec=project.data.get("policy_spec", DEFAULT_SPEC),
        prompt=project.data.get("policy_prompt", DEFAULT_PROMPT),
    )
    if not run["spec"].strip() or not run["prompt"].strip():
        raise ValueError("Selection criteria and prompt are required in /policy")
    project.data.setdefault("policy_runs", []).append(run)
    project.save()
    judge = runtime_factory(project.folder, model)
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
            step["candidates"] = [c["id"] for c in candidates]
            step["status"] = "selecting"
            state = dict(
                spec=run["spec"],
                candidates=[
                    dict(
                        node=c["id"],
                        parent=c["prompt"],
                        continuation=c["text"][len(c["prompt"]) :],
                    )
                    for c in candidates
                ],
            )
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
            step["decision"] = validate(result, candidates)
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
        raise
    finally:
        judge.close()
        run["finished"] = now()
        project.save()
    await emit(
        "operation", dict(stage=run["status"], message="Loom loops: " + run["status"])
    )
    return run
