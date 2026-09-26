import asyncio
import json

import pytest

from character_lab.domain import Project
from character_lab.policy import DEFAULT_PROMPT, grow, validate


def decision(candidates):
    return {
        "reviews": [
            {
                "node": n["id"],
                "decision": "explore",
                "reason": "Develops the image",
                "evidence": n["text"][len(n["prompt"]) :],
            }
            for n in candidates
        ],
        "selected": candidates[-1]["id"],
        "reason": "Most promising direction",
    }


def test_decision_must_cover_real_candidates_and_ground_quotes():
    candidates = [dict(id="a", text="Source new text", prompt="Source")]
    result = decision(candidates)
    assert validate(result, candidates)["selected"] == result["selected"]
    result["reviews"][0]["evidence"] = "invented"
    with pytest.raises(ValueError, match="evidence"):
        validate(result, candidates)
    result = decision(candidates)
    result["selected"] = "other"
    with pytest.raises(ValueError, match="selected"):
        validate(result, candidates)
    result = decision(candidates)
    result["reviews"] = []
    with pytest.raises(ValueError, match="every"):
        validate(result, candidates)


def test_quote_line_wrapping_is_resolved_to_exact_source_span():
    candidates = [dict(id="a", text="SourceA line\nlooks back.", prompt="Source")]
    result = decision(candidates)
    result["reviews"][0]["evidence"] = "A line looks back."
    validated = validate(result, candidates)["reviews"][0]
    assert validated["evidence"] == "A line\nlooks back."
    assert validated["reported_evidence"] == "A line looks back."
    assert (
        candidates[0]["text"][6:][
            validated["evidence_start"] : validated["evidence_end"]
        ]
        == validated["evidence"]
    )
    assert result["reviews"][0]["evidence"] == "A line looks back."


@pytest.mark.asyncio
async def test_bounded_growth_preserves_alternatives_and_separates_policy(tmp_path):
    model = tmp_path / "policy.gguf"
    model.touch()
    project = Project(tmp_path / "project")
    root = project.add("Seed.", kind="source")
    calls = []

    class Generator:
        model = {"name": "base"}
        process = object()

        async def stream(self, prompt, settings, trace):
            calls.append(("generate", prompt))
            trace["request"] = {"prompt": prompt}
            yield " A path opens."

        def close(self):
            calls.append(("close_generator",))

    class Judge:
        def __init__(self, *args):
            pass

        async def judge(self, messages, trace):
            assert calls[-1][0] == "close_generator"
            state = json.loads(messages[1]["content"])
            assert state["spec"] == "User selection criteria"
            calls.append(("judge",))
            candidates = [
                dict(id=c["node"], prompt="", text=c["continuation"])
                for c in state["candidates"]
            ]
            return decision(candidates)

        def close(self):
            calls.append(("close_judge",))

    run = await grow(
        project,
        root["id"],
        "Seed.",
        Generator(),
        {"path": str(model)},
        "User selection criteria",
        DEFAULT_PROMPT,
        2,
        2,
        {"n_predict": 32, "temperature": 1.0, "top_p": 0.98},
        lambda *args: None,
        Judge,
    )
    assert run["status"] == "complete" and len(run["steps"]) == 2
    nodes = [n for n in project.data["nodes"] if n["kind"] == "generated"]
    assert len(nodes) == 4 and not any(n["kept"] for n in nodes)
    assert all("User selection" not in n["prompt"] for n in nodes)
    assert run["steps"][1]["parent"] == run["steps"][0]["decision"]["selected"]
    assert nodes[2]["prompt"] == nodes[1]["text"]
    assert Project(project.folder).data["policy_runs"][0]["status"] == "complete"


@pytest.mark.asyncio
async def test_cancelled_policy_keeps_all_generated_branches(tmp_path):
    model = tmp_path / "policy.gguf"
    model.touch()
    p = Project(tmp_path / "project")
    root = p.add("Seed.", kind="source")

    class Generator:
        model = {}
        process = object()

        async def stream(self, *args):
            yield " new"

        def close(self):
            pass

    class Judge:
        def __init__(self, *args):
            pass

        async def judge(self, messages, trace):
            trace["request"] = messages
            raise asyncio.CancelledError()

        def close(self):
            pass

    with pytest.raises(asyncio.CancelledError):
        await grow(
            p,
            root["id"],
            "Seed.",
            Generator(),
            {"path": str(model)},
            "spec",
            "prompt",
            2,
            1,
            {},
            lambda *a: None,
            Judge,
        )
    loaded = Project(p.folder)
    assert loaded.data["policy_runs"][0]["status"] == "stopped"
    assert len(loaded.data["policy_runs"][0]["steps"][0]["trace"]["request"]) == 2
    assert all(n["status"] == "complete" for n in loaded.data["nodes"])
