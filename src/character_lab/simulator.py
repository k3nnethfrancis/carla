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

from . import monitor
from .domain import now
from .scheduling import parallel_map

CHARACTER_TEMPLATE = (
    "{anthology}\n\nFull conversation with Model C:\n\n{history}\n\n**Model C:**"
)
VISITOR_TEMPLATE = (
    "{visitor_brief}\n\nFull conversation with Model C:\n\n{history}\n\n**User:**"
)


def defaults(alias):
    sampling = dict(n_predict=512, temperature=1.0, top_p=0.98)
    return dict(
        monitor_mode="off",
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
    return config


def validate(config, project, validate_settings):
    if "token_range" in config:
        raise ValueError("Token ranges were removed; use a single maximum")
    if config["monitor_mode"] not in {"off", "jev", "diffusion"}:
        raise ValueError("Choose Off, Jev or DiffusionGemma monitoring")
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
            template.format(anthology="", history="", visitor_brief="")
        except (KeyError, ValueError, IndexError, AttributeError) as exc:
            raise ValueError(f"Invalid {role} template: {exc}") from exc
        if "{history}" not in template:
            raise ValueError(f"{role} template must include {{history}}")
    if "{anthology}" not in config["character_template"]:
        raise ValueError("Character template must include {anthology}")
    if not isinstance(config["visitor_brief"], str):
        raise ValueError("Visitor brief must be text")
    if not isinstance(config["opening"], str) or not config["opening"].strip():
        raise ValueError("Supply an opening user message")


def summary(run):
    names = {}
    for conversation in run["conversations"]:
        for turn in conversation["turns"]:
            if turn.get("model"):
                names.setdefault(turn["role"], turn["model"]["name"])
    return {key: run.get(key) for key in ("id", "status", "created")} | {
        "label": ("Opening preview · " if run.get("preview") else "")
        + " / ".join(
            names.get(role, run["config"][role + "_alias"])
            for role in ("character", "visitor")
        ),
        "count": run["config"]["conversations"],
        "parent": run.get("parent"),
        "conversations": [
            {"index": c["index"], "status": c["status"]} for c in run["conversations"]
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
        if k not in {"conversations", "documents", "config"}
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
                                if k not in {"request", "response"}
                            }
                            for check in turn.get("monitor_checks", [])
                        ],
                        "monitor": {
                            k: v
                            for k, v in turn.get("monitor", {}).items()
                            if k not in {"request", "response"}
                        },
                    }
                    for turn in conversation["turns"]
                ]
            }
            for conversation in run["conversations"]
        ]
    }


async def generate(project, config, runtime_factory, emit, seed=None):
    """Persist after each chunk; cancel/failure preserves partial output and provenance."""
    from .stream_monitor import TurnMonitor

    models = {m["alias"]: copy.deepcopy(m) for m in project.data["models"]}
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
        parent=seed["parent"] if seed else None,
    )
    project.data.setdefault("simulation_runs", []).append(run)
    project.save()
    runtime = None
    anthology = "\n\n".join(n["text"] for n in docs)
    for index in range(config["conversations"]):
        run["conversations"].append(
            dict(
                index=index,
                status="queued",
                turns=copy.deepcopy(seed["turns"])
                if seed
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
    try:
        # Batch independent conversations by speaker to avoid competing model loads.
        steps = [-1] if not seed and config["opening_mode"] == "generated" else []
        if not config.get("preview"):
            steps += list(range(config["turns"] * 2 - 1))
            if seed and seed["turns"] and seed["turns"][-1]["role"] == "character":
                steps.insert(
                    0, -2
                )  # Resume with the visitor, then requested character replies.

        async def sample(conversation, role, runtime, alias):
            speaker = "Model C" if role == "character" else "User"
            index = conversation["index"]
            conversation["status"] = "running"
            history = "\n\n".join(
                f"**{'Model C' if t['role'] == 'character' else 'User'}:** {t['text']}"
                for t in conversation["turns"]
            )
            prompt = (
                config["opening_prompt"]
                if role == "opening"
                else config[role + "_template"].format(
                    anthology=anthology,
                    history=history,
                    visitor_brief=config["visitor_brief"],
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
                    total=config["conversations"],
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
                                    total=config["conversations"],
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
            if not any(
                c["status"] in {"queued", "running"} for c in run["conversations"]
            ):
                break
            if runtime:
                if runtime.process is None:
                    raise ValueError(
                        "Stop the externally managed model server before switching simulator models"
                    )
                runtime.close()
            runtime = runtime_factory(project.folder, models[alias])

            async def schedule(data):
                await emit("capacity", data | {"model": alias})

            runtime.on_schedule = schedule

            async def advance(conversation):
                for role in segment:
                    if conversation["status"] not in {"queued", "running"}:
                        break
                    await sample(conversation, role, runtime, alias)
                if segment_index == len(segments) - 1 and conversation["status"] in {
                    "queued",
                    "running",
                }:
                    conversation["status"] = "complete"
                    project.save()
                    await emit("simulation", view(run))

            await parallel_map(run["conversations"], advance)

        for conversation in run["conversations"]:
            if conversation["status"] in {"queued", "running"}:
                conversation["status"] = "complete"
        if config.get("loops", 1) > 1 and runtime and runtime.process is None:
            raise ValueError(
                "Stop the externally managed generator before switching to selection"
            )
        run["status"] = "complete"
    except asyncio.CancelledError:
        run["status"] = "stopped"
    except Exception as exc:
        run.update(status="failed", error=str(exc))
    finally:
        for conversation in run["conversations"]:
            for turn in conversation["turns"]:
                if turn.get("monitor", {}).get("status") == "checking":
                    turn["monitor"]["status"] = "cancelled"
                if turn["status"] == "generating":
                    turn["status"] = run["status"]
            if conversation["status"] in {"queued", "running"}:
                conversation["status"] = run["status"]
        if runtime:
            runtime.close()
        run["finished"] = now()
        project.save()
        await emit("simulation", view(run))
    return run


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
        turns=copy.deepcopy(conversation["turns"]),
        documents=copy.deepcopy(run["documents"]),
        config=copy.deepcopy(run["config"]),
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
        parent=seed["parent"],
        conversations=[dict(index=0, status="draft", turns=turns)],
    )
    project.data.setdefault("simulation_runs", []).append(run)
    project.save()
    return run
