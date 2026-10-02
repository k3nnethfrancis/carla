"""Named operational policies project onto the existing generation configuration.

Turning a policy On selects it for inference and turns its peers Off. The internal
routing pointer remembers the last configuration when all policies are Off.
"""

import copy
import uuid

from . import assessments, credentials, monitor, policy_overrides, simulator
from .policy import DEFAULT_PROMPT, DEFAULT_SPEC

PURPOSES = {"monitoring", "selection"}


def enabled(purpose, config):
    return (
        config.get("monitor_mode", "off") != "off"
        if purpose == "monitoring"
        else config.get("selection_enabled", False)
    )


def disabled(purpose, config):
    value = copy.deepcopy(config)
    if purpose == "monitoring":
        if value.get("monitor_mode") in {"jev", "diffusion"}:
            value["monitor_provider"] = value["monitor_mode"]
        value["monitor_mode"] = "off"
    else:
        value["selection_enabled"] = False
    return value


def turn_off_peers(project, purpose, keep=None):
    for item in project.data["operational_policies"][purpose]:
        if item["id"] != keep and enabled(purpose, item["config"]):
            store(item, disabled(purpose, expanded(item)))


def apply(session, purpose, item, config):
    """Only an On edit changes routing; Off edits cannot select an unused policy."""
    routes = session.project.data["active_operational_policies"]
    if enabled(purpose, config):
        turn_off_peers(session.project, purpose, item["id"])
        routes[purpose] = item["id"]
    if routes.get(purpose) == item["id"]:
        project_config(session, purpose, config)


def model_key(model):
    return model.get("alias") or model.get("name", "")


def reconcile_model(project, model):
    """Only the active policy follows an explicit configured selector change."""
    active = project.data.get("active_operational_policies", {}).get("selection")
    selected = next(
        (
            p
            for p in project.data.get("operational_policies", {}).get("selection", [])
            if p["id"] == active
        ),
        None,
    )
    if selected and selected["config"].get("model_alias") != model_key(model):
        config = expanded(selected)
        config["model_alias"] = model_key(model)
        store(selected, config)
        project.data["selection_model_alias"] = model_key(model)


def current(project, purpose, alias, policy_model):
    if purpose == "monitoring":
        return {
            k: copy.deepcopy(v)
            for k, v in simulator.configuration(project, alias).items()
            if k.startswith("monitor_") and k != "monitor_policy"
        }
    return dict(
        selection_enabled=project.data.get("selection_enabled", False),
        selection_assessment_prompt=project.data.get(
            "selection_assessment_prompt", assessments.DEFAULT_PROMPT
        ),
        selection_call_mode=project.data.get("selection_call_mode", "separate"),
        policy_spec=project.data.get("policy_spec", DEFAULT_SPEC),
        policy_prompt=project.data.get("policy_prompt", DEFAULT_PROMPT),
        selection_behaviors=copy.deepcopy(
            project.data.get(
                "selection_behaviors",
                [
                    dict(
                        id="criteria",
                        name="Selection criteria",
                        spec=project.data.get("policy_spec", DEFAULT_SPEC),
                        enabled=True,
                    )
                ],
            )
        ),
        model_alias=project.data.get("selection_model_alias", model_key(policy_model)),
    )


def store(item, config):
    """Actions belong to the policy; config is the judge's assessment settings."""
    value = copy.deepcopy(config)
    if "monitor_dimensions" in value:
        item["actions"] = {}
        for behavior in value["monitor_dimensions"]:
            item["actions"][behavior["id"]] = {
                k: behavior.pop(k) for k in ("action", "color") if k in behavior
            }
    else:
        item["actions"] = {"advance_selected": True}
    item["config"] = value
    item["revision"] = item.get("revision", 0) + 1


def expanded(item):
    """Materialize execution/editor defaults without rewriting saved revisions."""
    value = copy.deepcopy(item["config"])
    if "selection_enabled" in value:
        value.setdefault("selection_assessment_prompt", assessments.DEFAULT_PROMPT)
        value.setdefault("selection_call_mode", "separate")
    for behavior in value.get("monitor_dimensions", []):
        behavior.update(item.get("actions", {}).get(behavior["id"], {}))
    return value


def summaries(project):
    """Wire projection keeps old setting editors working; persisted actions are singular."""
    return {
        purpose: [{**copy.deepcopy(item), "config": expanded(item)} for item in items]
        for purpose, items in project.data.get("operational_policies", {}).items()
    }


def migrate(project, alias, policy_model):
    catalog = project.data.setdefault("operational_policies", {})
    active = project.data.setdefault("active_operational_policies", {})
    for purpose in sorted(PURPOSES):
        if purpose not in catalog:
            item = dict(
                id=uuid.uuid4().hex[:12],
                name="Default policy",
                config=current(project, purpose, alias, policy_model),
            )
            store(item, item["config"])
            catalog[purpose] = [item]
            active[purpose] = item["id"]
    for purpose in PURPOSES:
        # Rename only the former built-in label, never a user name or a collision.
        if not any(item["name"] == "Default policy" for item in catalog[purpose]):
            legacy = next(
                (item for item in catalog[purpose] if item["name"] == "Default"), None
            )
            if legacy:
                legacy["name"] = "Default policy"
                legacy["revision"] += 1
        if not any(item["id"] == active.get(purpose) for item in catalog[purpose]):
            turn_off_peers(project, purpose)
            fallback = catalog[purpose][0] if catalog[purpose] else None
            active[purpose] = fallback["id"] if fallback else ""
            config = (
                expanded(fallback)
                if fallback
                else current(project, purpose, alias, policy_model)
            )
            project_values(project, purpose, disabled(purpose, config))
        turn_off_peers(project, purpose, active.get(purpose))
    if "behavior_library" not in project.data:
        project.data["behavior_library"] = []
        seen = set()
        behaviors = monitor.dimensions(project.data.get("simulator_config", {}))
        behaviors += [
            b
            for p in project.data.get("evaluation_policies", [])
            for b in p.get("behaviors", [])
        ]
        behaviors.append(
            dict(
                name="Selection criteria",
                spec=project.data.get("policy_spec", DEFAULT_SPEC),
            )
        )
        for b in behaviors:
            key = (b["name"], b["spec"])
            if key not in seen:
                project.data["behavior_library"].append(
                    dict(
                        id=uuid.uuid4().hex[:12],
                        name=b["name"],
                        spec=b["spec"],
                        revision=1,
                    )
                )
                seen.add(key)


def sync(session, purpose):
    p = session.project
    key = p.data["active_operational_policies"].get(purpose)
    item = next(
        (x for x in p.data["operational_policies"][purpose] if x["id"] == key), None
    )
    if item is None:
        item = dict(id=uuid.uuid4().hex[:12], name="Default policy")
        p.data["operational_policies"][purpose].append(item)
        p.data["active_operational_policies"][purpose] = item["id"]
        store(
            item,
            current(p, purpose, session.runtime.model["alias"], session.policy_model),
        )
    if item:
        config = current(
            p, purpose, session.runtime.model["alias"], session.policy_model
        )
        if expanded(item) != config:
            store(item, config)
        if enabled(purpose, config):
            turn_off_peers(p, purpose, item["id"])


def validate(session, purpose, config, *, activating=False):
    key = "monitor_dimensions" if purpose == "monitoring" else "selection_behaviors"
    if isinstance(config.get(key), list):
        for behavior in config[key]:
            if isinstance(behavior, dict) and not behavior.get("id"):
                behavior["id"] = "custom_" + uuid.uuid4().hex[:12]
    if purpose == "monitoring":
        if any(not k.startswith("monitor_") for k in config):
            raise ValueError("Monitoring policies contain only monitor settings")
        merged = {
            **simulator.configuration(session.project, session.runtime.model["alias"]),
            **config,
        }
        policy_overrides.remember_provider(merged, config)
        simulator.validate(merged, session.project, session.validate_settings)
        if (
            activating
            and merged["monitor_mode"] == "jev"
            and not credentials.openrouter_key()[0]
        ):
            raise ValueError(
                "Configure an OpenRouter API key before turning on Jev monitoring"
            )
        return {
            k: v
            for k, v in merged.items()
            if k.startswith("monitor_") and k != "monitor_policy"
        }
    config.setdefault("selection_assessment_prompt", assessments.DEFAULT_PROMPT)
    config.setdefault("selection_call_mode", "separate")
    if set(config) != {
        "selection_enabled",
        "selection_assessment_prompt",
        "selection_call_mode",
        "policy_spec",
        "policy_prompt",
        "model_alias",
        "selection_behaviors",
    }:
        raise ValueError("Selection policies configure enabled, spec, prompt and model")
    behaviors = config["selection_behaviors"]
    if not isinstance(behaviors, list) or any(
        not isinstance(b, dict)
        or not isinstance(b.get("id"), str)
        or not isinstance(b.get("name"), str)
        or not isinstance(b.get("spec"), str)
        or not b["spec"].strip()
        or type(b.get("enabled", True)) is not bool
        for b in behaviors
    ):
        raise ValueError("Selection behaviors need names, specs and enabled states")
    if len({b["id"] for b in behaviors}) != len(behaviors):
        raise ValueError("Selection behavior IDs must be unique")
    for behavior in behaviors:
        behavior.setdefault("expected", "present")
        behavior.setdefault("threshold", 0.8)
        if behavior["expected"] not in {"present", "absent"}:
            raise ValueError(
                "Choose whether the selection behavior should be present or absent"
            )
        if (
            type(behavior["threshold"]) not in (int, float)
            or not 0 < behavior["threshold"] <= 1
        ):
            raise ValueError("Behavior threshold must be greater than 0 and at most 1")
    if config["selection_call_mode"] not in {"separate", "bundled"}:
        raise ValueError("Choose Separate or Bundled call mode")
    enabled = [b for b in behaviors if b.get("enabled", True)]
    if config["selection_enabled"] and not enabled:
        raise ValueError("Enable at least one selection behavior")
    config["policy_spec"] = (
        "\n\n".join(
            f"{b['name']} (expected {b['expected']}): {b['spec']}" for b in enabled
        )
        or config["policy_spec"]
    )
    if type(config["selection_enabled"]) is not bool:
        raise ValueError("Selection enabled must be boolean")
    if any(
        not isinstance(config[k], str) or not config[k].strip()
        for k in ("policy_spec", "policy_prompt", "selection_assessment_prompt")
    ):
        raise ValueError("Selection instructions cannot be empty")
    model = session.judge_model(config["model_alias"])
    if config["selection_enabled"]:
        from .exploration import require_selector

        require_selector(model)
    return copy.deepcopy(config)


def project_config(session, purpose, config):
    project_values(session.project, purpose, config)


def project_values(project, purpose, config):
    if purpose == "monitoring":
        project.data.setdefault("simulator_config", {}).update(copy.deepcopy(config))
    else:
        project.data.update(
            {k: copy.deepcopy(v) for k, v in config.items() if k != "model_alias"}
        )
        project.data["selection_model_alias"] = config["model_alias"]


async def dispatch(session, command, args, request_id):
    p = session.project
    if command.startswith("behavior."):
        items = p.data["behavior_library"]
        old = next((b for b in items if b["id"] == args.get("id")), None)
        if args.get("id") and old is None:
            raise ValueError("Behavior spec not found")
        if command == "behavior.delete":
            if old is None:
                raise ValueError("Choose a behavior spec")
            items.remove(old)
        elif command == "behavior.save":
            if any(
                not isinstance(args.get(k), str) or not args[k].strip()
                for k in ("name", "spec")
            ):
                raise ValueError("Behavior needs a name and spec")
            value = dict(
                id=old["id"] if old else uuid.uuid4().hex[:12],
                name=args["name"].strip(),
                spec=args["spec"],
                revision=old["revision"] + 1 if old else 1,
            )
            if old:
                items[items.index(old)] = value
            else:
                items.append(value)
        else:
            raise ValueError("Unknown behavior command")
    else:
        purpose = args.get("purpose")
        if purpose not in PURPOSES:
            raise ValueError("Choose monitoring or selection")
        items = p.data["operational_policies"][purpose]
        old = next((x for x in items if x["id"] == args.get("id")), None)
        if args.get("id") and old is None:
            raise ValueError("Policy not found")
        active = p.data["active_operational_policies"]
        if command == "operational.policy.save":
            name = args.get("name", "")
            if not isinstance(name, str) or not name.strip():
                raise ValueError("Name the policy")
            if any(x["name"] == name.strip() and x is not old for x in items):
                raise ValueError("A policy already has that name")
            updates = args.get("config", {})
            if not isinstance(updates, dict):
                raise ValueError("Policy config must be an object")
            config = {
                **(
                    expanded(old)
                    if old
                    else current(
                        p, purpose, session.runtime.model["alias"], session.policy_model
                    )
                ),
                **updates,
            }
            if old is None:
                config = disabled(purpose, config)
            config = validate(
                session, purpose, config, activating=enabled(purpose, config)
            )
            if old:
                old.update(name=name.strip())
                store(old, config)
            else:
                old = dict(id=uuid.uuid4().hex[:12], name=name.strip(), config=config)
                store(old, config)
                items.append(old)
                if not any(item["id"] == active.get(purpose) for item in items):
                    active[purpose] = old["id"]
            apply(session, purpose, old, config)
        elif command == "operational.policy.activate":
            if old is None:
                raise ValueError("Choose a policy")
            config = expanded(old)
            if purpose == "monitoring":
                config["monitor_mode"] = (
                    config.get("monitor_provider", "off")
                    if config.get("monitor_mode") == "off"
                    else config["monitor_mode"]
                )
                if config["monitor_mode"] == "off":
                    raise ValueError(
                        "Choose a monitoring model before turning this policy On"
                    )
            else:
                config["selection_enabled"] = True
            config = validate(session, purpose, config, activating=True)
            store(old, config)
            apply(session, purpose, old, config)
        elif command == "operational.policy.delete":
            if old is None:
                raise ValueError("Choose a policy")
            routed = active.get(purpose) == old["id"]
            items.remove(old)
            if routed:
                fallback = items[0] if items else None
                config = disabled(purpose, expanded(fallback or old))
                if fallback:
                    store(fallback, config)
                active[purpose] = fallback["id"] if fallback else ""
                project_config(session, purpose, config)
        else:
            raise ValueError("Unknown policy command")
    p.save()
    await session.snapshot(request_id)
