"""Policy-owned judges and behaviors; flatten only at the execution boundary."""

import copy
import uuid

from .monitor import LOCAL_URL, local_url


def uid():
    return uuid.uuid4().hex[:12]


def legacy(definition):
    """Copy a legacy evaluator into a private judge, avoiding shared-edit surprises."""
    return {
        **{
            k: copy.deepcopy(definition[k])
            for k in ("id", "name", "kind", "model", "prompt", "revision", "endpoint")
            if k in definition
        },
        "call_mode": "separate",
        "behaviors": [
            {
                k: copy.deepcopy(definition[k])
                for k in ("id", "name", "spec", "threshold", "revision")
            }
            | {"enabled": True}
        ],
    }


def normalize(judges, previous=()):
    if not isinstance(judges, list):
        raise ValueError("Policy judges must be a list")
    old = {j["id"]: j for j in previous if isinstance(j, dict)}
    result = []
    seen = set()
    for source in judges:
        if not isinstance(source, dict):
            raise ValueError("Choose a judge with behaviors")
        j = {
            k: copy.deepcopy(source.get(k))
            for k in ("id", "name", "kind", "model", "prompt", "call_mode")
        }
        j["id"] = j["id"] or uid()
        if j["id"] in seen:
            raise ValueError("Judge identifiers must be unique within a policy")
        seen.add(j["id"])
        for key in ("name", "model"):
            if not isinstance(j[key], str) or not j[key].strip():
                raise ValueError(f"Judge needs {key}")
        if j["kind"] not in {"llm", "jev", "diffusion"}:
            raise ValueError("Choose an LLM, Jev or DiffusionGemma judge")
        j["call_mode"] = j["call_mode"] or "separate"
        if j["call_mode"] not in {"separate", "bundled"}:
            raise ValueError("Call mode must be separate or bundled")
        if not isinstance(j["prompt"], str) or (
            j["kind"] == "llm" and not j["prompt"].strip()
        ):
            raise ValueError("LLM judge needs a prompt")
        if j["kind"] == "diffusion":
            j["endpoint"] = local_url(source.get("endpoint", LOCAL_URL))
        j["behaviors"] = []
        behaviors = source.get("behaviors", [])
        if not isinstance(behaviors, list):
            raise ValueError("Judge behaviors must be a list")
        prior = old.get(j["id"], {})
        prior_behaviors = {b["id"]: b for b in prior.get("behaviors", [])}
        behavior_ids = set()
        for b in behaviors:
            if not isinstance(b, dict):
                raise ValueError("Invalid behavior")
            value = {k: copy.deepcopy(b.get(k)) for k in ("name", "spec", "threshold")}
            value.update(id=b.get("id") or uid(), enabled=b.get("enabled", True))
            if "source_id" in b:
                if (
                    not isinstance(b["source_id"], str)
                    or type(b.get("source_revision")) is not int
                    or b["source_revision"] < 1
                ):
                    raise ValueError("Invalid copied behavior provenance")
                value.update(
                    source_id=b["source_id"], source_revision=b["source_revision"]
                )
            if value["id"] in behavior_ids:
                raise ValueError("Behavior identifiers must be unique within a judge")
            behavior_ids.add(value["id"])
            if any(
                not isinstance(value[k], str) or not value[k].strip()
                for k in ("name", "spec")
            ):
                raise ValueError("Behavior needs a name and spec")
            if type(value["enabled"]) is not bool:
                raise ValueError("Behavior enabled must be boolean")
            if (
                type(value["threshold"]) not in (int, float)
                or not 0 < value["threshold"] <= 1
            ):
                raise ValueError("Pass threshold must be greater than 0 and at most 1")
            old_b = prior_behaviors.get(value["id"], {})
            value["revision"] = old_b.get("revision", 0) + (
                any(old_b.get(k) != v for k, v in value.items())
            )
            j["behaviors"].append(value)
        j["revision"] = prior.get("revision", 0) + any(
            prior.get(k) != v for k, v in j.items()
        )
        result.append(j)
    return result


def flatten(policy):
    definitions = []
    for judge in policy["judges"]:
        if judge.get("missing"):
            raise ValueError(
                "A legacy judge is missing; remove or configure it in Policies"
            )
        for behavior in judge["behaviors"]:
            if behavior.get("enabled", True):
                definitions.append(
                    {
                        **copy.deepcopy(behavior),
                        **{
                            k: copy.deepcopy(judge[k])
                            for k in (
                                "kind",
                                "model",
                                "prompt",
                                "endpoint",
                                "call_mode",
                            )
                            if k in judge
                        },
                        "id": judge["id"] + ":" + behavior["id"],
                        "behavior_id": behavior["id"],
                        "judge_id": judge["id"],
                        "judge_name": judge["name"],
                        "judge_revision": judge["revision"],
                    }
                )
    return definitions
