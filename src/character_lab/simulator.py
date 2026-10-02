"""Raw-document conversation sampling. Traces are authored by code, not the LLM.

This alternating-seat sampler is a local reconstruction, not a claim to recover
Computer's unpublished initial sampler. No character weights are trained here.
"""

import asyncio
import copy
import hashlib
import random
import uuid
from contextlib import aclosing
from itertools import groupby

from . import monitor, simulator_actions, templates
from .domain import now
from .scheduling import parallel_map
from .simulator_names import NAME_FIELDS

CHARACTER_TEMPLATE = (
    "{{anthology}}\n\nFull conversation with Model C:\n\n{{history}}\n\n**Model C:**"
)
VISITOR_TEMPLATE = (
    "{{visitor_brief}}\n\nFull conversation with Model C:\n\n{{history}}\n\n**User:**"
)


def defaults(alias):
    sampling = dict(n_predict=512, temperature=1.0, top_p=0.98)
    return dict(
        monitor_mode="off",
        monitor_call_mode="separate",
        monitor_interval_tokens=512,
        monitor_after_reply=True,
        monitor_during_reply=True,
        monitor_model="jev-latest",
        monitor_local_url=monitor.LOCAL_URL,
        monitor_local_model=monitor.LOCAL_MODEL,
        monitor_dimensions=monitor.dimensions({}),
        documents=[],
        character_alias=alias,
        visitor_alias=alias,
        opening="What would you like to talk about?",
        opening_mode="fixed",
        opening_alias="",
        opening_prompt="Write one opening message from a curious person beginning a conversation. Choose a concrete topic or situation. Output only their message.\n\nOpening message:",
        opening_settings=dict(n_predict=128, temperature=1.0, top_p=0.98),
        visitor_brief="A curious visitor talks with Model C.",
        conversations=1,
        turns=4,
        character_settings=sampling.copy(),
        visitor_settings=sampling.copy(),
        character_template=CHARACTER_TEMPLATE,
        visitor_template=VISITOR_TEMPLATE,
    )


def configuration(project, alias):
    config = {**defaults(alias), **project.data.get("simulator_config", {})}
    config["monitor_dimensions"] = monitor.dimensions(
        project.data.get("simulator_config", {})
    )
    config.pop("monitor_questions", None)
    key = project.data.get("active_operational_policies", {}).get("monitoring")
    named = next(
        (
            p
            for p in project.data.get("operational_policies", {}).get("monitoring", [])
            if p["id"] == key
        ),
        None,
    )
    if named:
        config["monitor_policy"] = {k: named[k] for k in ("id", "name", "revision")}
    return config


def validate(config, project, validate_settings):
    if "token_range" in config:
        raise ValueError("Token ranges were removed; use a single maximum")
    if config["monitor_mode"] not in {"off", "jev", "diffusion"}:
        raise ValueError("Choose Off, Jev or DiffusionGemma monitoring")
    if config.get("monitor_call_mode", "separate") not in {"separate", "bundled"}:
        raise ValueError("Choose Separate or Bundled monitor calls")
    if config["monitor_mode"] == "diffusion":
        monitor.local_url(config.get("monitor_local_url", monitor.LOCAL_URL))
        if (
            not isinstance(config.get("monitor_local_model"), str)
            or not config["monitor_local_model"].strip()
        ):
            raise ValueError("Supply a local monitor model")
    if (
        not isinstance(config["monitor_model"], str)
        or not config["monitor_model"].strip()
    ):
        raise ValueError("Supply a monitor model")
    monitor.validate_dimensions(monitor.dimensions(config))
    for key in ("monitor_after_reply", "monitor_during_reply"):
        if type(config.get(key, True)) is not bool:
            raise ValueError("Monitor timing switches must be boolean")
    interval = config.get("monitor_interval_tokens", 512)
    if type(interval) is not int or interval < 0:
        raise ValueError(
            "In-progress check interval must be a nonnegative token count (0 = end only)"
        )
    aliases = {m["alias"] for m in project.data["models"]}
    if (
        config["character_alias"] not in aliases
        or config["visitor_alias"] not in aliases
    ):
        raise ValueError("Choose configured local base models for both speakers")
    for key in ("conversations", "turns"):
        if type(config[key]) is not int or config[key] < 1:
            raise ValueError(f"{key} must be a positive integer")
    if not isinstance(config["documents"], list) or not all(
        isinstance(key, str) for key in config["documents"]
    ):
        raise ValueError("Select anthology document IDs")
    if len(set(config["documents"])) != len(config["documents"]):
        raise ValueError("Select unique anthology document versions")
    known = {n["id"] for n in project.data["nodes"] if n["kept"]}
    if any(key not in known for key in config["documents"]):
        raise ValueError("Selected document is no longer in the anthology")
    if config["opening_mode"] not in {"fixed", "generated"}:
        raise ValueError("Opening mode must be fixed or generated")
    if config["opening_alias"] and config["opening_alias"] not in aliases:
        raise ValueError("Choose a configured opening model")
    if (
        not isinstance(config["opening_prompt"], str)
        or not config["opening_prompt"].strip()
    ):
        raise ValueError("Supply an opening generation prompt")
    for role in ("character", "visitor", "opening"):
        sampling = config[role + "_settings"]
        if not isinstance(sampling, dict) or set(sampling) != {
            "n_predict",
            "temperature",
            "top_p",
        }:
            raise ValueError("Sampling supports output tokens, temperature and top-p")
        validate_settings(dict(count=1, rounds=1, **sampling))
        if role == "opening":
            continue
        template = config[role + "_template"]
        if not isinstance(template, str):
            raise ValueError(f"{role} template must be text")
        try:
            templates.conversation(
                template, dict(anthology="", history="", visitor_brief="")
            )
        except (KeyError, ValueError, IndexError, AttributeError) as exc:
            raise ValueError(f"Invalid {role} template: {exc}") from exc
        has_history = "history" in templates.conversation_variables(template)[0]
        if not has_history:
            raise ValueError(role + " template must include {{history}}")
    character_template = config["character_template"]
    has_anthology = (
        "anthology" in templates.conversation_variables(character_template)[0]
    )
    if not has_anthology:
        raise ValueError("Character template must include {{anthology}}")
    if not isinstance(config["visitor_brief"], str):
        raise ValueError("Visitor brief must be text")
    if not isinstance(config["opening"], str) or not config["opening"].strip():
        raise ValueError("Supply an opening user message")


def summary(run):
    names = {}
    for conversation in run["conversations"]:
        for turn in conversation["turns"]:
            if turn.get("model"):
                names[turn["role"]] = turn["model"]["name"]
    return {key: run.get(key) for key in ("id", "status", "created")} | {
        "models_label": ("Opening preview · " if run.get("preview") else "")
        + " / ".join(
            names.get(role, run["config"][role + "_alias"])
            for role in ("character", "visitor")
        ),
        "count": run["config"]["conversations"],
        "parent": run.get("parent"),
        **{
            key: run[key]
            for key in (
                "alternative_group",
                "alternative_index",
                "alternative_count",
                "source_set",
                "source_scope",
                "alternative_scope",
                "revision",
                "action",
                *NAME_FIELDS,
                "operation_label",
                "operation_short_label",
                "operation_title",
                "alternative_label",
                "alternative_short_label",
                "alternative_title",
            )
            if key in run
        },
        "conversations": [
            {key: c[key] for key in ("index", "status", *NAME_FIELDS) if key in c}
            | {"turn_count": len(c["turns"])}
            for c in run["conversations"]
        ],
        "policy_stops": sum(
            c["status"] == "policy_stopped" for c in run["conversations"]
        ),
    }


def view(run):
    """Only the selected conversation text is sent to the TUI; raw requests stay on disk."""
    return {
        k: v
        for k, v in run.items()
        if k not in {"conversations", "documents", "config", "revisions"}
    } | {
        "conversations": [
            {k: v for k, v in conversation.items() if k != "turns"}
            | {
                "turns": [
                    {
                        k: v
                        for k, v in turn.items()
                        if k not in {"trace", "prompt", "monitor", "monitor_checks"}
                    }
                    | {
                        "monitor_checks": [
                            {
                                k: v
                                for k, v in check.items()
                                if k not in {"request", "response", "calls"}
                            }
                            for check in turn.get("monitor_checks", [])
                        ],
                        "monitor": {
                            k: v
                            for k, v in turn.get("monitor", {}).items()
                            if k not in {"request", "response", "calls"}
                        },
                    }
                    for turn in conversation["turns"]
                ]
            }
            for conversation in run["conversations"]
        ]
    }


def new_run(project, config, seed=None, metadata=None):
    docs = (
        seed["documents"]
        if seed
        else [project.node(key) for key in config["documents"]]
    )
    run = dict(
        id=uuid.uuid4().hex[:12],
        created=now(),
        status="running",
        protocol="alternating-raw-documents-v1",
        preview=config.get("preview", False),
        scheduling="bounded-concurrent-model-segments-v2",
        config=copy.deepcopy(config),
        documents=[
            dict(
                node=n.get("id", n.get("node")),
                text=n["text"],
                sha256=hashlib.sha256(n["text"].encode()).hexdigest(),
            )
            for n in docs
        ],
        conversations=[],
        parent=copy.deepcopy(seed["parent"]) if seed else None,
        parent_revision=seed.get("source_revision", 0) if seed else None,
        action=config.get("action", "loom"),
    )
    if config.get("source_scope"):
        run["source_scope"] = copy.deepcopy(config["source_scope"])
    run.update(metadata or {})
    project.data.setdefault("simulation_runs", []).append(run)
    for index in range(config["conversations"]):
        ancestor = (
            seed["conversations"][index] if seed and "conversations" in seed else seed
        )
        run["conversations"].append(
            dict(
                index=index,
                status="queued",
                parent=copy.deepcopy(ancestor["parent"]) if ancestor else None,
                parent_revision=ancestor.get("source_revision", 0)
                if ancestor
                else None,
                turns=copy.deepcopy(ancestor["turns"])
                if ancestor
                else (
                    []
                    if config["opening_mode"] == "generated"
                    else [
                        dict(
                            role="user",
                            text=config["opening"],
                            origin="opening",
                            status="complete",
                        )
                    ]
                ),
            )
        )
    project.save()
    return run, run["conversations"]


async def generate(project, config, runtime_factory, emit, seed=None):
    """Legacy single-set sampler; action planning lives in generate_alternatives."""
    return (
        await _generate(project, config, runtime_factory, emit, [(config, seed, {})])
    )[0]


async def generate_alternatives(project, config, runtime_factory, emit, seed=None):
    """Run all selected sets through one bounded, model-segmented scheduler."""
    if "action" not in config:
        return [await generate(project, config, runtime_factory, emit, seed)]
    sets = simulator_actions.seed_sets(seed)
    config = copy.deepcopy(config)
    if seed is not None and config.get("alternatives", 1) == 1:
        config["action"] = "continue"
    continuing = config["action"] == "continue"
    count = 1 if continuing else config.get("alternatives", 1)
    if not continuing and seed is None:
        local = copy.deepcopy(config)
        local["conversations"] = count
        return await _generate(
            project, config, runtime_factory, emit, [(local, seed, {})]
        )
    config["source_scope"] = seed.get("scope") or simulator_actions.scope_for_sets(sets)
    group = simulator_actions.alternative_id()
    plans = []
    for alternative in range(count):
        for source_index, source in enumerate(sets):
            local = copy.deepcopy(config)
            local["conversations"] = (
                len(simulator_actions.members(source)) if source else 1
            )
            metadata = (
                {}
                if continuing
                else dict(
                    alternative_group=group,
                    alternative_index=alternative,
                    alternative_count=count,
                    source_set=source_index,
                )
            )
            plans.append((local, source, metadata))
    return await _generate(project, config, runtime_factory, emit, plans)


def conversation_prompt(config, role, documents, turns):
    """One rendering path for fit checks and actual speaker completions."""
    return templates.conversation(
        config[role + "_template"],
        dict(
            anthology="\n\n".join(n["text"] for n in documents),
            history="\n\n".join(
                f"**{'Model C' if t['role'] == 'character' else 'User'}:** {t['text']}"
                for t in turns
            ),
            visitor_brief=config["visitor_brief"],
        ),
    )


async def _generate(project, config, runtime_factory, emit, plans):
    """Persist each leaf's chunks under its real run/index, sharing loaded weights."""
    from .stream_monitor import TurnMonitor

    models = {m["alias"]: copy.deepcopy(m) for m in project.data["models"]}
    runs, work = [], []
    # Validate every frozen source before archiving or creating any destination.
    for _, source, _ in plans:
        simulator_actions.validate_seed(project, source)
    runtime = None
    # A fixed fresh opening is fully known before creating any run. Reuse the
    # loaded character model after checking, rather than loading weights twice.
    if (
        all(source is None for _, source, _ in plans)
        and config["opening_mode"] == "fixed"
        and not config.get("preview")
    ):
        runtime = runtime_factory(project.folder, models[config["character_alias"]])
        try:
            documents = [project.node(key) for key in config["documents"]]
            prompt = conversation_prompt(
                config,
                "character",
                documents,
                [dict(role="user", text=config["opening"])],
            )
            await runtime.preflight(prompt, config["character_settings"])
        except BaseException:
            runtime.close()
            raise
    try:
        for local, seed, metadata in plans:
            if config.get("action") == "continue":
                run, selected = simulator_actions.continue_run(project, local, seed)
            else:
                run, selected = new_run(project, local, seed, metadata)
            run.update(metadata)
            runs.append(run)
            for conversation in selected:
                work.append((run, conversation))
        if config.get("action") != "continue" and any(
            metadata for _, _, metadata in plans
        ):
            # Scope is supplied independently of the scheduler's flattened run batches.
            sources = [
                source
                for _, source, _ in plans[
                    : len(plans) // max(1, config.get("alternatives", 1))
                ]
            ]
            frozen = sources[0] if len(sources) == 1 else {"sets": sources}
            if config.get("source_scope"):
                frozen = dict(frozen, scope=config["source_scope"])
            simulator_actions.attach_scopes(runs, frozen)
        project.save()
    except BaseException:
        if runtime:
            runtime.close()
        raise
    # Every set is either fresh or resumed; target resolution disallows mixing.
    seed = plans[0][1]
    try:
        for run in runs:
            await emit("simulation", view(run))
        # Batch independent conversations by speaker to avoid competing model loads.
        steps = [-1] if not seed and config["opening_mode"] == "generated" else []
        if not config.get("preview"):
            steps += list(range(config["turns"] * 2 - 1))
            if seed:
                steps.insert(
                    0, -2
                )  # Resume with the visitor, then requested character replies.

        async def sample(run, conversation, role, runtime, alias):
            speaker = "Model C" if role == "character" else "User"
            index = conversation["index"]
            conversation["status"] = "running"
            prompt = (
                config["opening_prompt"]
                if role == "opening"
                else conversation_prompt(
                    config, role, run["documents"], conversation["turns"]
                )
            )
            settings = {
                **config[role + "_settings"],
                "seed": random.randrange(2**31),
                "stop": [
                    "\n\n**User:**" if speaker == "Model C" else "\n\n**Model C:**"
                ],
            }
            turn = dict(
                role="user" if role == "opening" else role,
                text="",
                origin="generated_opening" if role == "opening" else "generated",
                status="generating",
                model=copy.deepcopy(models[alias]),
                settings=settings,
                prompt=prompt,
                trace={},
            )
            conversation["turns"].append(turn)
            project.save()
            await emit("simulation", view(run))
            await emit(
                "simulation.progress",
                dict(
                    run=run["id"],
                    conversation=index + 1,
                    total=len(run["conversations"]),
                    role=role,
                    stage="loading / waiting",
                ),
            )
            async with TurnMonitor(
                config, project, run, conversation, turn, emit
            ) as watcher:
                async with aclosing(
                    runtime.stream(prompt, settings, turn["trace"])
                ) as stream:
                    first_chunk = True
                    while True:
                        try:
                            chunk = await watcher.next_chunk(stream)
                        except StopAsyncIteration:
                            break
                        if first_chunk:
                            await emit(
                                "simulation.progress",
                                dict(
                                    run=run["id"],
                                    conversation=index + 1,
                                    total=len(run["conversations"]),
                                    role=role,
                                    stage="generating",
                                ),
                            )
                            first_chunk = False
                        turn["text"] += chunk
                        project.stream_delta(
                            {
                                "run": run["id"],
                                "conversation": index,
                                "turn": len(conversation["turns"]) - 1,
                            },
                            chunk,
                            turn["trace"],
                        )
                        await emit(
                            "simulation.token",
                            dict(
                                run=run["id"],
                                conversation=index,
                                turn=len(conversation["turns"]) - 1,
                                text=chunk,
                            ),
                        )
                        watcher.checkpoint()
                        if watcher.stopped:
                            break
                await watcher.finish()
            # Keep unmodified output and observations. Completion is a transport
            # status, not a quality verdict; observations never stop later turns.
            events = turn["trace"].get("events", [])
            final = events[-1] if events else {}
            flags = []
            if not turn["text"].strip():
                flags.append("empty")
            if "**User:**" in turn["text"] or "**Model C:**" in turn["text"]:
                flags.append("unexpected_role_boundary")
            if final.get("stop_type") == "limit" or final.get("stopped_limit"):
                flags.append("token_limit")
            turn.update(
                status="policy_stopped" if watcher.stopped else "complete",
                flags=flags,
                raw_span=[0, len(turn["text"])],
            )
            project.save()
            await emit("simulation", view(run))

        roles = [
            "opening"
            if step == -1
            else "character"
            if step >= 0 and step % 2 == 0
            else "visitor"
            for step in steps
        ]
        # Same-model speakers advance independently per conversation. Model changes
        # remain barriers so only one set of GPU weights is resident at a time.
        segments = [
            (alias, list(grouped))
            for alias, grouped in groupby(
                roles,
                key=lambda role: config[role + "_alias"] or config["visitor_alias"],
            )
        ]
        for segment_index, (alias, segment) in enumerate(segments):
            if not any(c["status"] in {"queued", "running"} for _, c in work):
                break
            if runtime and segment_index > 0:
                if runtime.process is None:
                    raise ValueError(
                        "Stop the externally managed model server before switching simulator models"
                    )
                runtime.close()
                runtime = None
            if runtime is None:
                runtime = runtime_factory(project.folder, models[alias])

            async def schedule(data):
                await emit("capacity", data | {"model": alias})

            runtime.on_schedule = schedule

            async def advance(item):
                run, conversation = item
                for step_index, role in enumerate(segment):
                    # Batch members may end on different speakers (e.g. an interrupted reply).
                    if (
                        seed
                        and segment_index == 0
                        and step_index == 0
                        and role == "visitor"
                        and (
                            not conversation["turns"]
                            or conversation["turns"][-1]["role"] != "character"
                        )
                    ):
                        continue
                    if conversation["status"] not in {"queued", "running"}:
                        break
                    await sample(run, conversation, role, runtime, alias)
                if segment_index == len(segments) - 1 and conversation["status"] in {
                    "queued",
                    "running",
                }:
                    conversation["status"] = "complete"
                    project.save()
                    await emit("simulation", view(run))

            await parallel_map(work, advance)

        for _, conversation in work:
            if conversation["status"] in {"queued", "running"}:
                conversation["status"] = "complete"
        if config.get("loops", 1) > 1 and runtime and runtime.process is None:
            raise ValueError(
                "Stop the externally managed generator before switching to selection"
            )
        for run in runs:
            run["status"] = "complete"
    except asyncio.CancelledError:
        for run in runs:
            run["status"] = "stopped"
    except Exception as exc:
        for run in runs:
            run.update(status="failed", error=str(exc))
    finally:
        for run, conversation in work:
            for turn in conversation["turns"]:
                if turn.get("monitor", {}).get("status") == "checking":
                    turn["monitor"]["status"] = "cancelled"
                if turn["status"] == "generating":
                    turn["status"] = run["status"]
            if conversation["status"] in {"queued", "running"}:
                conversation["status"] = run["status"]
        if runtime:
            runtime.close()
        for run in runs:
            run["finished"] = now()
        project.save()
        for run in runs:
            await emit("simulation", view(run))
    return runs


def conversation_seed(project, run_id, index):
    run = next(
        (r for r in project.data.get("simulation_runs", []) if r["id"] == run_id), None
    )
    if (
        run is None
        or type(index) is not int
        or not 0 <= index < len(run["conversations"])
    ):
        raise ValueError("Select a saved conversation")
    conversation = run["conversations"][index]
    if run["status"] == "running":
        raise ValueError("Stop the active run before forking or continuing")
    return dict(
        parent=dict(run=run_id, conversation=index),
        source_revision=run.get("revision", 0),
        turns=copy.deepcopy(conversation["turns"]),
        documents=copy.deepcopy(run["documents"]),
        config=copy.deepcopy(run["config"]),
    )


def batch_seed(project, run_id, indices=None):
    """Freeze each sibling independently; continuation never clones one winner."""
    run = next(
        (r for r in project.data.get("simulation_runs", []) if r["id"] == run_id), None
    )
    if run is None or not run["conversations"]:
        raise ValueError("Select a saved Loom")
    if indices is None:
        indices = [c["index"] for c in run["conversations"]]
    if (
        not isinstance(indices, list)
        or not indices
        or any(type(i) is not int for i in indices)
        or len(set(indices)) != len(indices)
    ):
        raise ValueError("Select a nonempty set of distinct conversations")
    seeds = [conversation_seed(project, run_id, index) for index in indices]
    return dict(
        parent=dict(run=run_id, conversation=-1),
        source_revision=run.get("revision", 0),
        documents=seeds[0]["documents"],
        conversations=seeds,
    )


def fork_conversation(project, seed, args):
    """Edits fork through that turn; downstream replies never survive changed context."""
    turns = seed["turns"]
    if "text" in args:
        text = args["text"]
        if not isinstance(text, str) or not text.strip():
            raise ValueError("Write a message before saving")
        if args.get("visitor"):
            if turns and turns[-1]["role"] != "character":
                raise ValueError(
                    "Edit the existing visitor message before adding another"
                )
            turns.append(
                dict(role="visitor", text=text, origin="human", status="complete")
            )
        else:
            index = args.get("turn")
            if type(index) is not int or not 0 <= index < len(turns):
                raise ValueError("Select a turn to edit")
            turns = turns[:index] + [
                dict(
                    role=turns[index]["role"],
                    text=text,
                    origin="human_edit",
                    status="complete",
                    source=dict(**seed["parent"], turn=index),
                )
            ]
    config = seed["config"]
    config["conversations"] = 1
    run = dict(
        id=uuid.uuid4().hex[:12],
        created=now(),
        status="draft",
        protocol="conversation-fork-v1",
        config=config,
        documents=seed["documents"],
        parent=copy.deepcopy(seed["parent"]),
        parent_revision=seed.get("source_revision", 0),
        conversations=[dict(index=0, status="draft", turns=turns)],
    )
    project.data.setdefault("simulation_runs", []).append(run)
    project.save()
    return run


def fork_sets(project, seed, args):
    """Copy selected sets and their ancestry without inference or source mutation."""
    simulator_actions.validate_seed(project, seed)
    sets = simulator_actions.seed_sets(seed)
    if "text" in args:
        leaves = [leaf for item in sets for leaf in simulator_actions.members(item)]
        if len(leaves) != 1:
            raise ValueError("Select one conversation to edit")
        return [fork_conversation(project, copy.deepcopy(leaves[0]), args)]
    group = simulator_actions.alternative_id()
    runs = []
    for source_index, item in enumerate(sets):
        leaves = simulator_actions.members(item)
        config = copy.deepcopy(leaves[0]["config"])
        config["conversations"] = len(leaves)
        run = dict(
            id=uuid.uuid4().hex[:12],
            created=now(),
            status="draft",
            protocol="conversation-fork-v1",
            config=config,
            documents=copy.deepcopy(item["documents"]),
            parent=copy.deepcopy(item["parent"]),
            parent_revision=item.get("source_revision", 0),
            conversations=[
                dict(
                    index=i,
                    status="draft",
                    turns=copy.deepcopy(leaf["turns"]),
                    parent=copy.deepcopy(leaf["parent"]),
                    parent_revision=leaf.get("source_revision", 0),
                )
                for i, leaf in enumerate(leaves)
            ],
        )
        if len(sets) > 1:
            run.update(
                alternative_group=group,
                alternative_index=0,
                alternative_count=1,
                source_set=source_index,
            )
        runs.append(run)
    simulator_actions.attach_scopes(runs, seed)
    project.data.setdefault("simulation_runs", []).extend(runs)
    project.save()
    return runs
