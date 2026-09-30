"""Resolve per-operation policy switches without editing workspace defaults."""

import copy

from . import credentials
from .exploration import require_selector


def selection(args, saved, action, count, model):
    enabled = args.get("selection", saved)
    if type(enabled) is not bool:
        raise ValueError("Selection must be on or off")
    if args.get("selection") is True:
        if action != "loom" or count < 2:
            raise ValueError("Selection needs Loom with at least 2 alternatives")
        if "loops" not in args:
            raise ValueError(
                "Selection needs --loops 1 or more to generate and judge alternatives"
            )
    active = enabled and action == "loom" and count > 1 and "loops" in args
    if active:
        require_selector(model)
    return active


def monitoring(config, args):
    resolved = copy.deepcopy(config)
    if "monitoring" in args:
        enabled = args["monitoring"]
        if type(enabled) is not bool:
            raise ValueError("Monitoring must be on or off")
        if not enabled:
            resolved["monitor_mode"] = "off"
        elif resolved.get("monitor_mode", "off") == "off":
            provider = resolved.get("monitor_provider")
            if provider not in {"jev", "diffusion"}:
                raise ValueError(
                    "Choose a monitoring provider in /policy first, then use --monitoring on"
                )
            resolved["monitor_mode"] = provider
    if (
        args.get("monitoring") is True
        and resolved.get("monitor_mode") == "jev"
        and not credentials.openrouter_key()[0]
    ):
        raise ValueError(
            "Configure an OpenRouter API key in /policy → Monitoring first"
        )
    return resolved


def remember_provider(config, previous):
    """Off disables execution but retains the user's explicit provider choice."""
    mode = config.get("monitor_mode", "off")
    if mode in {"jev", "diffusion"}:
        config["monitor_provider"] = mode
    elif previous.get("monitor_mode") in {"jev", "diffusion"}:
        config["monitor_provider"] = previous["monitor_mode"]
