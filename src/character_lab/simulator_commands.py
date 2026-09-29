"""Simulator commands: configuration, monitor dimensions and conversation views.

Session retains workspace locking and job ownership; this module handles the
Simulator's command arguments and emits its domain events through that session.
"""

import copy
from uuid import uuid4

from . import credentials, evaluation_sets, monitor, simulator
from .exploration import require_selector


async def dispatch(session, command, args, request_id):
    p = session.project
    if command == "loom-policy.key":
        credentials.save_openrouter_key(args.get("key"))
        config = simulator.configuration(p, session.runtime.model["alias"])
        config["monitor_mode"] = "jev"
        p.data["simulator_config"] = config
        p.save()
        await session.snapshot(request_id)
        return
    if command.startswith("loom-policy."):
        config = simulator.configuration(p, session.runtime.model["alias"])
        items = monitor.dimensions(config)
        if command == "loom-policy.add":
            items.append(
                dict(
                    id="custom_" + uuid4().hex,
                    name=args["name"],
                    spec=args["spec"],
                    enabled=args.get("enabled", True),
                    action=args.get("action", "warn"),
                    color=args.get("color", "amber"),
                    decision=args.get("decision", "most_likely"),
                    threshold=args.get("threshold", 0.8),
                )
            )
        else:
            item = next((d for d in items if d["id"] == args["id"]), None)
            if item is None:
                raise ValueError("Dimension not found")
            if command == "loom-policy.delete":
                if item["id"] in monitor.BUILTINS:
                    raise ValueError("Default dimensions cannot be deleted")
                items.remove(item)
            elif command == "loom-policy.update":
                item.update({k: v for k, v in args.items() if k != "id"})
            else:
                raise ValueError("Unknown policy operation")
        args = {"monitor_dimensions": items}
        command = "simulator.configure"
    if command == "simulator.configure":
        if args.get("monitor_mode") == "jev" and not credentials.openrouter_key()[0]:
            raise ValueError(
                "Configure an OpenRouter API key in /policy → Monitoring first"
            )
        config = {**simulator.configuration(p, session.runtime.model["alias"]), **args}
        simulator.validate(config, p, session.validate_settings)
        p.data["simulator_config"] = config
        p.save()
        await session.snapshot(request_id)
        return
    if command == "simulator.fork":
        seed = simulator.conversation_seed(p, args["run"], args["conversation"])
        run = simulator.fork_conversation(p, seed, args)
        await session.snapshot(request_id)
        await session.emit(
            "simulation",
            simulator.view(run)
            | {"opened": True, "forked": True, "open_conversation": 0},
            request_id,
        )
        return
    if command == "simulator.open":
        run = next(
            (r for r in p.data.get("simulation_runs", []) if r["id"] == args["run"]),
            None,
        )
        if run is None:
            raise ValueError("Simulation run not found")
        index = args.get("conversation")
        if index is not None and (
            type(index) is not int or not 0 <= index < len(run["conversations"])
        ):
            raise ValueError("Conversation not found")
        await session.emit(
            "simulation",
            simulator.view(run)
            | {"opened": True, "open_conversation": args.get("conversation")},
            request_id,
        )
        return
    if command == "simulator.inspect":
        run = next(
            r for r in p.data.get("simulation_runs", []) if r["id"] == args["run"]
        )
        record = dict(run)
        record["selection"] = next(
            (
                r
                for r in p.data.get("policy_runs", [])
                if r["id"] == run.get("policy_run")
            ),
            None,
        )
        await session.emit("inspection", record, request_id)
        return
    if command in {"simulator.run", "simulator.preview"}:
        config = copy.deepcopy(
            simulator.configuration(p, session.runtime.model["alias"])
        )
        seed = None
        if command == "simulator.run" and "run" in args:
            seed = (
                simulator.conversation_seed(p, args["run"], args["conversation"])
                if "conversation" in args
                else simulator.batch_seed(p, args["run"], args.get("conversations"))
            )
            config[
                "documents"
            ] = []  # The frozen ancestor anthology is carried by seed.
        if "message" in args:
            message = args["message"]
            if not isinstance(message, str) or not message.strip():
                raise ValueError("Supply a nonempty visitor opening message")
            if seed:
                raise ValueError(
                    "Clear the conversation selection before supplying an opening message"
                )
            config.update(opening=message, opening_mode="fixed")
        if command == "simulator.preview":
            config.update(
                preview=True,
                opening_mode="generated",
                conversations=3,
                documents=[],
            )
        if "count" in args:
            config["conversations"] = args["count"]
        if seed and "conversations" in seed:
            size = len(seed["conversations"])
            if "count" in args and args["count"] != size:
                raise ValueError(
                    "A group continuation advances every conversation once; select one conversation to create alternatives"
                )
            config["conversations"] = size
        if "turns" in args:
            config["turns"] = args["turns"]
        if "n_predict" in args:
            for role in ("character", "visitor"):
                config[role + "_settings"]["n_predict"] = args["n_predict"]
        config["loops"] = args.get("loops", 1)
        if type(config["loops"]) is not int or config["loops"] < 1:
            raise ValueError("Loops must be a positive integer")
        if config["loops"] > 1:
            require_selector(session.policy_model)
        simulator.validate(config, p, session.validate_settings)
        if not config["documents"] and not config.get("preview") and not seed:
            raise ValueError("Select at least one anthology document in Simulator")
        await session.start_simulation(
            config,
            seed,
            request_id,
            evaluation_sets.plan(session, args["eval"]) if args.get("eval") else None,
        )
        return
    raise ValueError("Unknown simulator command: " + command)
