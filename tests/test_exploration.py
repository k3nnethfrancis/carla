"""Selection assesses full candidates before the distinct holistic choice."""

import copy
import json

import pytest

from character_lab.domain import Project
from character_lab.exploration import explore


@pytest.mark.asyncio
@pytest.mark.parametrize("call_mode", ["separate", "bundled"])
async def test_selection_requires_every_behavior_then_preserves_holistic_choice(
    tmp_path, call_mode
):
    project = Project(tmp_path / "workspace")
    model_file = tmp_path / "judge.gguf"
    model_file.touch()
    model = dict(alias="judge", path=str(model_file))
    project.data.update(
        selection_call_mode=call_mode,
        policy_prompt="A saved chooser template that must remain intact.",
        selection_behaviors=[
            dict(
                id="coherent",
                name="Coherence",
                spec="Coherent text",
                expected="present",
            ),
            dict(id="harmful", name="Harm", spec="Harmful text", expected="absent"),
        ],
    )
    calls = []

    class Judge:
        def __init__(self, *args):
            pass

        async def judge(self, messages, trace):
            payload = json.loads(messages[1]["content"])
            calls.append(payload)
            if "candidates" in payload:
                # Both good candidates passed; ranking still chooses the second.
                assert [c["node"] for c in payload["candidates"]] == ["good", "better"]
                assert messages[0]["content"] == project.data["policy_prompt"]
                return dict(
                    reviews=[
                        dict(
                            node=c["node"],
                            decision="explore",
                            reason="Useful",
                            evidence=c["continuation"],
                        )
                        for c in payload["candidates"]
                    ],
                    selected="better",
                    reason="Better development",
                )

            def result(spec):
                observed = "Harmful" not in spec or "bad" in payload["text"]
                return dict(
                    passed=observed,
                    reason="Observed in full text",
                    evidence=payload["text"],
                )

            if "behaviors" in payload:
                return {
                    "results": {
                        b["id"]: result(b["criteria"]) for b in payload["behaviors"]
                    }
                }
            return result(payload["criteria"])

        def close(self):
            pass

    candidates = [
        dict(id=key, prompt="parent ", text="parent " + key)
        for key in ("bad", "good", "better")
    ]
    frozen = copy.deepcopy(candidates)
    advanced = []

    async def batch(*args):
        return candidates

    async def advance(key):
        advanced.append(key)

    async def emit(*args):
        pass

    run = await explore(project, 1, model, Judge, batch, advance, emit)
    assert run["status"] == "complete" and advanced == ["better"]
    assert len(calls) == (4 if call_mode == "bundled" else 7)
    step = run["steps"][0]
    assert step["inputs"] == frozen
    assert step["eligible"] == ["good", "better"]
    assert step["assessments"][0]["records"][1]["result"]["passed"] is False
    candidates[0]["text"] = "changed after judging"
    project.data["selection_behaviors"][0]["spec"] = "changed after judging"
    assert step["inputs"] == frozen
    assert run["behaviors"][0]["spec"] == "Coherent text"
    assert all(
        r["definition"]["scope"] == "whole" and r["trace"]["messages"]
        for a in step["assessments"]
        for r in a["records"]
    )


@pytest.mark.asyncio
async def test_failed_assessments_never_advance_or_become_negative_judgments(tmp_path):
    project = Project(tmp_path / "workspace")
    model_file = tmp_path / "judge.gguf"
    model_file.touch()

    class Judge:
        def __init__(self, *args):
            pass

        async def judge(self, messages, trace):
            raise ValueError("context window exceeded")

        def close(self):
            pass

    async def batch(*args):
        return [dict(id="a", prompt="", text="A full candidate")]

    async def advance(key):
        pytest.fail("A failed assessment cannot advance")

    async def emit(*args):
        pass

    with pytest.raises(ValueError, match="assessments failed"):
        await explore(
            project, 1, dict(path=str(model_file)), Judge, batch, advance, emit
        )
    run = project.data["policy_runs"][-1]
    record = run["steps"][0]["assessments"][0]["records"][0]
    assert record["status"] == "failed" and "result" not in record
    assert record["error"] == "context window exceeded"
    assert run["status"] == "failed" and run["selected"] is None


@pytest.mark.asyncio
async def test_cancellation_stops_pending_behavior_calls_and_retains_complete_results(
    tmp_path,
):
    import asyncio

    project = Project(tmp_path / "workspace")
    path = tmp_path / "judge.gguf"
    path.touch()
    project.data["selection_behaviors"] = [
        dict(id=str(i), name=str(i), spec=str(i)) for i in range(3)
    ]

    class Judge:
        def __init__(self, *args):
            self.calls = 0

        async def judge(self, messages, trace):
            self.calls += 1
            if self.calls == 2:
                raise asyncio.CancelledError()
            return dict(passed=True, reason="Present", evidence="candidate")

        def close(self):
            pass

    async def batch(*args):
        return [dict(id="a", prompt="", text="candidate")]

    async def unused(*args):
        pass

    with pytest.raises(asyncio.CancelledError):
        await explore(project, 1, dict(path=str(path)), Judge, batch, unused, unused)
    run = project.data["policy_runs"][-1]
    records = run["steps"][0]["assessments"][0]["records"]
    assert [r["status"] for r in records] == ["complete", "stopped", "stopped"]
    assert records[0]["result"]["passed"] is True
    assert run["status"] == "stopped"
