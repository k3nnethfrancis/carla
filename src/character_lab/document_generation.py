"""Document action orchestration: freeze scope, generate leaves, select whole sets.

Legacy transport callers remain in Session. New Continue/Loom requests use this
path so document identity, set shape and operation overrides have one owner.
"""

import asyncio
from dataclasses import replace

from . import document_actions, evaluation_sets, policy_overrides, simulator
from .exploration import explore


async def start(session, args, request_id, eval_plan):
    from .service import DEFAULT_SETTINGS

    p = session.project
    action = args["action"]
    if action not in {"continue", "loom"}:
        raise ValueError("Choose Continue or Loom")
    if any(k in args for k in ("visitor", "message", "turns", "visitor_model")):
        raise ValueError(
            "Visitor messages, visitor models and turns apply only to Simulator"
        )
    loops = args.get("loops", 1)
    if type(loops) is not int or loops < 1:
        raise ValueError("Loops must be a positive integer")
    if action == "continue" and "count" in args:
        raise ValueError(
            "Continue advances each selected item; use Loom for alternatives"
        )
    alias = args.get("model", session.runtime.model["alias"])
    model = next((m for m in p.data["models"] if m["alias"] == alias), None)
    if model is None:
        raise ValueError("Unknown generator model; choose one in /config")
    settings = {**DEFAULT_SETTINGS, **p.data.get("settings", {})}
    if "n_predict" in args:
        settings["n_predict"] = args["n_predict"]
    session.validate_settings(settings)
    count = args.get("count", 1)
    if type(count) is not int or count < 1:
        raise ValueError("Alternatives must be a positive integer")
    selection = policy_overrides.selection(
        args,
        p.data.get("selection_enabled", False),
        action,
        count,
        session.policy_model,
    )
    policy_config = policy_overrides.monitoring(simulator.configuration(p, alias), args)
    policy_config["selection_enabled"] = selection
    policy_config["loops"] = loops
    targets = [key for key in ("refs", "node", "nodes", "set", "scope") if key in args]
    if len(targets) > 1:
        raise ValueError(
            "Choose one document target: passages, document versions or a set"
        )
    if "offset" in args and "offsets" in args:
        raise ValueError("Supply one document position")
    if "refs" in args:
        if any(key in args for key in ("offset", "offsets", "text")):
            raise ValueError(
                "Open source passages in Branches before choosing a document position"
            )
        if not isinstance(args["refs"], list) or any(
            not isinstance(ref, str) for ref in args["refs"]
        ):
            raise ValueError("Select source passages first")
        valid = {
            s["key"] + ":" + x["id"] for s in session.sources for x in s["passages"]
        }
        if not args["refs"] or any(ref not in valid for ref in args["refs"]):
            raise ValueError("Select source passages first")
        node = p.source_root(session.sources, args["refs"])
        nodes = [node["id"]]
    elif args.get("set") or "scope" in args:
        nodes = None
    else:
        if "nodes" in args:
            nodes = args["nodes"]
            if not isinstance(nodes, list) or not nodes:
                raise ValueError("Select at least one document")
        elif "node" in args:
            if not isinstance(args["node"], str) or not args["node"]:
                raise ValueError("Select a saved document")
            nodes = [args["node"]]
        else:
            nodes = [session.current()["id"]]
        if "text" in args:
            if len(nodes) != 1:
                raise ValueError("Save edits before generating a document set")
            original = p.node(nodes[0])
            if args["text"] != original["text"]:
                raise ValueError("Save or cancel the document edit before generating")
    offsets = args.get("offsets")
    if "offset" in args:
        if not nodes or len(nodes) != 1:
            raise ValueError("A cursor position requires one selected document")
        offsets = {nodes[0]: args["offset"]}
    plan = document_actions.plan(
        p,
        action,
        nodes,
        count=count,
        offsets=offsets,
        set_id=args.get("set"),
        scope=args.get("scope"),
        fork=args.get("from_anthology", False),
    )
    session.job_id = request_id
    session.job = asyncio.create_task(
        run(session, plan, settings, loops, model, eval_plan, selection, policy_config)
    )


async def run(
    session, plan, settings, loops, model, eval_plan, selection, policy_config
):
    p = session.project
    original_runtime = session.runtime
    generated = []
    current_plan = plan

    async def emit(kind, data):
        await session.emit(kind, data, session.job_id)

    async def batch(policy_id=None, index=0, plans=None):
        candidates = []
        for item in plans or [current_plan]:
            for candidate in document_actions.begin(p, item):
                candidates.append(replace(candidate, index=len(candidates)))
        outputs = await session.generate(
            "continue",
            None,
            "",
            settings,
            len(candidates),
            nested=True,
            candidates=candidates,
            policy_config=policy_config,
        )
        generated.extend(outputs)
        if policy_id:
            if session.runtime.process is None:
                raise ValueError(
                    "Stop the externally managed generator before switching to selection"
                )
            session.runtime.close()
            for node in outputs:
                node.update(policy_run=policy_id, loop=index + 1)
            p.save()
        groups = {}
        for node in outputs:
            groups.setdefault(node["document_set"], []).append(node)
        return [
            dict(
                id=key,
                prompt="",
                text="\n\n".join(
                    f"Document {i + 1}\nSource:\n{node['prompt']}\nContinuation:\n{node['text'][len(node['prompt']) :]}"
                    for i, node in enumerate(
                        sorted(nodes, key=lambda n: n["set_member"])
                    )
                ),
            )
            for key, nodes in groups.items()
            if all(n["status"] == "complete" for n in nodes)
        ]

    async def advance(key):
        nonlocal current_plan
        current_plan = document_actions.plan(p, "loom", count=plan.count, set_id=key)

    try:
        if original_runtime.model["alias"] != model["alias"]:
            original_runtime.close()
            session.runtime = session.runtime_factory(p.folder, model)
        if loops == 1 and not selection:
            await batch()
        elif not selection:
            # Split once, then advance every resulting alternative. Loops extend
            # each future; they do not multiply the fan-out at every step.
            groups = await batch()
            for _ in range(1, loops):
                plans = [
                    document_actions.plan(p, "continue", set_id=result["id"])
                    for result in groups
                ]
                if not plans:
                    break
                groups = await batch(plans=plans)
        else:
            await explore(
                p,
                loops,
                session.policy_model,
                session.runtime_factory,
                batch,
                advance,
                emit,
            )
        if eval_plan and not asyncio.current_task().cancelling():
            await evaluation_sets.after_generation(
                session,
                eval_plan,
                [{"node": n["id"]} for n in generated if n.get("status") == "complete"],
            )
    except asyncio.CancelledError:
        pass
    except Exception as exc:
        await emit("error", dict(message=str(exc)))
    finally:
        if session.runtime is not original_runtime:
            session.runtime.close()
            session.runtime = original_runtime
        session.job = None
        await session.snapshot()
