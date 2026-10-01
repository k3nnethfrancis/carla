"""Simulator commands: configuration, monitor dimensions and conversation views.

Session retains workspace locking and job ownership; this module handles the
Simulator's command arguments and emits its domain events through that session.
"""

import copy
from uuid import uuid4

from . import (
    credentials,
    evaluation_sets,
    monitor,
    operational_policies,
    policy_overrides,
    simulator,
    simulator_actions,
)
from .exploration import require_selector


async def dispatch(session, command, args, request_id):
    p = session.project
    if command == "loom-policy.key":
        credentials.save_openrouter_key(args.get("key"))
        if args.get("activate", True) is False:
            if args.get("policy_id"):
                saved = next(
                    (
                        x
                        for x in p.data["operational_policies"]["monitoring"]
                        if x["id"] == args["policy_id"]
                    ),
                    None,
                )
                if saved is None:
                    raise ValueError("Policy not found")
                await operational_policies.dispatch(
                    session,
                    "operational.policy.save",
                    dict(
                        purpose="monitoring",
                        id=saved["id"],
                        name=saved["name"],
                        config={
                            "monitor_mode": "off"
                            if args.get("preserve_disabled")
                            else "jev",
                            "monitor_provider": "jev",
                        },
                    ),
                    request_id,
                )
                return
            await session.snapshot(request_id)
            return
        config = simulator.configuration(p, session.runtime.model["alias"])
        config["monitor_mode"] = "jev"
        config["monitor_provider"] = "jev"
        p.data["simulator_config"] = config
        operational_policies.sync(session, "monitoring")
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
        policy_overrides.remember_provider(config, p.data.get("simulator_config", {}))
        simulator.validate(config, p, session.validate_settings)
        p.data["simulator_config"] = config
        operational_policies.sync(session, "monitoring")
        p.save()
        await session.snapshot(request_id)
        return
    if command == "simulator.rename":
        from . import simulator_names

        simulator_names.rename(p.data, args)
        p.save()
        await session.snapshot(request_id)
        for run in p.data.get("simulation_runs", []):
            await session.emit("simulation", simulator.view(run), request_id)
        return
    if command == "simulator.fork":
        seed = simulator_actions.resolve_seed(p, args)
        if seed is None:
            raise ValueError("Select a conversation or set to branch")
        runs = simulator.fork_sets(p, seed, args)
        await session.snapshot(request_id)
        for run in runs:
            await session.emit(
                "simulation",
                simulator.view(run)
                | {
                    "opened": True,
                    "forked": True,
                    "open_conversation": 0 if len(run["conversations"]) == 1 else None,
                },
                request_id,
            )
        return
    if command == "simulator.open":
        if "scope" in args:
            from . import action_scope

            scope = action_scope.validate(args["scope"], "conversation")
            runs = {}
            for leaf in action_scope.leaves(scope):
                run = next(
                    (
                        r
                        for r in p.data.get("simulation_runs", [])
                        if r["id"] == leaf.get("run")
                    ),
                    None,
                )
                index = leaf.get("conversation")
                if (
                    run is None
                    or type(index) is not int
                    or not 0 <= index < len(run["conversations"])
                ):
                    raise ValueError("Select a saved conversation")
                runs[run["id"]] = run
            for run in runs.values():
                await session.emit(
                    "simulation",
                    simulator.view(run)
                    | {
                        "opened": True,
                        "grid_group": scope.get("id", ""),
                    },
                    request_id,
                )
            return
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
        action = args.get("action")
        if action is None and any(key in args for key in ("selection", "monitoring")):
            action = "loom"
        if action is not None and action not in {"continue", "loom"}:
            raise ValueError("Choose Continue or Loom")
        if action == "continue" and "count" in args:
            raise ValueError("Continue advances the selection; count belongs to Loom")
        seed = (
            simulator_actions.resolve_seed(p, args)
            if command == "simulator.run"
            else None
        )
        if "documents" in args:
            config["documents"] = copy.deepcopy(args["documents"])
        if seed:
            config["documents"] = []  # Frozen ancestor anthology travels with the seed.
        if action == "continue" and seed is None:
            raise ValueError("Select a saved conversation or set to continue")
        supplied = [args[key] for key in ("visitor", "message", "msg") if key in args]
        if supplied:
            if any(value != supplied[0] for value in supplied):
                raise ValueError("Supply one visitor message")
            message = supplied[0]
            if not isinstance(message, str) or not message.strip():
                raise ValueError("Supply a nonempty visitor message")
            if seed and action is None and "visitor" not in args:
                raise ValueError(
                    "Clear the conversation selection before supplying an opening message"
                )
            if seed:
                seed = simulator_actions.add_visitor(seed, message)
            else:
                config.update(opening=message, opening_mode="fixed")
        if "model" in args:
            config["character_alias"] = args["model"]
        if "visitor_model" in args:
            config["visitor_alias"] = args["visitor_model"]
        if action is not None:
            config["action"] = action
            config["alternatives"] = args.get("count", 1)
            if type(config["alternatives"]) is not int or config["alternatives"] < 1:
                raise ValueError("Alternatives must be a positive integer")
        if command == "simulator.preview":
            config.update(
                preview=True,
                opening_mode="generated",
                conversations=3,
                documents=[],
            )
        if "count" in args:
            config["conversations"] = args["count"]
        if seed and "conversations" in seed and action is None:
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
        config["selection_enabled"] = policy_overrides.selection(
            args,
            p.data.get("selection_enabled", False),
            action or "loom",
            config.get("alternatives", config["conversations"]),
            session.policy_model,
            behaviors=p.data.get("selection_behaviors"),
        )
        config = policy_overrides.monitoring(config, args)
        if action is None and config["loops"] > 1:
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
