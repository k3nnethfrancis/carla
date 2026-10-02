"""Policy-owned judges and behaviors; flatten only at the execution boundary."""

import copy
import uuid

from . import templates
from .assessments import expected_state
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
            | (
                {"expected": expected_state(definition)}
                if "expected" in definition
                else {}
            )
        ],
    }


def lift_behaviors(policy):
    """Migrate editable nested policies only; never touch frozen run snapshots.

    Identical configs sharing an ID collapse. Conflicting IDs keep both variants,
    identifying the former owning judge in the renamed variant.
    """
    if not isinstance(policy.get("judges"), list) or any(
        not isinstance(j, dict) for j in policy["judges"]
    ):
        raise ValueError("Policy judges must be objects")
    if not isinstance(policy.get("behaviors", []), list):
        raise ValueError("Policy behaviors must be a list")
    if "behaviors" in policy and not any("behaviors" in j for j in policy["judges"]):
        return False
    behaviors = copy.deepcopy(policy.get("behaviors", []))
    used = {b["id"]: b for b in behaviors}
    # Compare original variants before collision renaming changes their identity.
    originals = copy.deepcopy(behaviors)
    for judge in policy["judges"]:
        nested = judge.pop("behaviors", [])
        if not isinstance(nested, list) or any(not isinstance(b, dict) for b in nested):
            raise ValueError("Behaviors must be objects")
        for source in nested:
            b = copy.deepcopy(source)
            b["id"] = b.get("id") or uid()
            existing = used.get(b["id"])
            if b in originals:
                continue
            originals.append(copy.deepcopy(b))
            if existing is not None:
                b["id"] = uid()
                b["name"] += " · " + judge["name"]
            used[b["id"]] = b
            behaviors.append(b)
    policy["behaviors"] = behaviors
    return True


def normalize(judges, previous=()):
    if not isinstance(judges, list):
        raise ValueError("Policy judges must be a list")
    old = {j["id"]: j for j in previous if isinstance(j, dict)}
    result = []
    seen = set()
    for source in judges:
        if not isinstance(source, dict):
            raise ValueError("Choose a judge configuration")
        j = {
            k: copy.deepcopy(source.get(k))
            for k in ("id", "name", "kind", "model", "prompt", "call_mode")
        }
        j["id"] = j["id"] or uid()
        if j["id"] in seen:
            raise ValueError("Judge identifiers must be unique within a policy")
        seen.add(j["id"])
        # Name is a compatibility field, derived from model for new saves.
        j["name"] = j["model"]
        for key in ("model",):
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
        if j["kind"] == "llm":
            templates.validate_assessment(j["prompt"])
        if j["kind"] == "diffusion":
            j["endpoint"] = local_url(source.get("endpoint", LOCAL_URL))
        prior = old.get(j["id"], {})
        j["revision"] = prior.get("revision", 0) + any(
            prior.get(k) != v for k, v in j.items()
        )
        result.append(j)
    return result


def normalize_behaviors(behaviors, previous=()):
    if not isinstance(behaviors, list):
        raise ValueError("Policy behaviors must be a list")
    prior_behaviors = {b["id"]: b for b in previous}
    behavior_ids = set()
    result = []
    for b in behaviors:
        if not isinstance(b, dict):
            raise ValueError("Invalid behavior")
        value = {k: copy.deepcopy(b.get(k)) for k in ("name", "spec", "threshold")}
        value.update(
            id=b.get("id") or uid(),
            enabled=b.get("enabled", True),
        )
        expected = expected_state(b)
        if "expected" in b:
            value["expected"] = expected
        if "source_id" in b:
            if (
                not isinstance(b["source_id"], str)
                or type(b.get("source_revision")) is not int
                or b["source_revision"] < 1
            ):
                raise ValueError("Invalid copied behavior provenance")
            value.update(source_id=b["source_id"], source_revision=b["source_revision"])
        if value["id"] in behavior_ids:
            raise ValueError("Behavior identifiers must be unique within a policy")
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
        result.append(value)
    return result


def flatten(policy):
    definitions = []
    for judge in policy["judges"]:
        if judge.get("missing"):
            raise ValueError(
                "A legacy judge is missing; remove or configure it in Policies"
            )
        for behavior in policy["behaviors"]:
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
                        "judge_name": judge["model"],
                        "judge_revision": judge["revision"],
                    }
                )
    return definitions
