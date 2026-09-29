"""Offline contracts for the optional standalone evaluation harness."""

import importlib.util
import json
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1] / "evals" / "monitoring"
sys.path.insert(0, str(ROOT))
spec = importlib.util.spec_from_file_location("monitoring_eval", ROOT / "run.py")
eval_run = importlib.util.module_from_spec(spec)
spec.loader.exec_module(eval_run)
from metrics import agreement, distribution, summarize  # noqa: E402


def fixture():
    return dict(
        questions={
            key: dict(type="noul", instructions=key)
            for key in ["looping", "spiraling", "harmful_language"]
        },
        cases=[
            dict(
                id="none",
                history=[dict(role="character", text="A coherent reply.")],
                expected=dict(looping=False, spiraling=False, harmful_language=False),
            )
        ],
    )


def test_request_matches_application_scan_suffix():
    f = fixture()
    request = eval_run.request_for(f, f["cases"][0], "model", list(f["questions"]))
    assert len(request["questions"]) == 3
    assert request["state"]["latest_message_complete"] is True
    assert request["state"]["history"] == f["cases"][0]["history"]
    assert (
        request["questions"]["looping"]["instructions"] == "looping" + eval_run.SUFFIX
    )


def test_plan_is_reproducible_and_checks_all_questions():
    f = fixture()
    plan = eval_run.plan(f, ["local", "hosted"], ["bundled", "separate"], 2, 17)
    assert plan == eval_run.plan(f, ["local", "hosted"], ["bundled", "separate"], 2, 17)
    assert len(plan) == 16
    for provider in ["local", "hosted"]:
        for repeat in range(2):
            one = [
                r
                for r in plan
                if r["provider"] == provider
                and r["repeat"] == repeat
                and r["mode"] == "separate"
            ]
            assert {r["questions"][0] for r in one} == set(f["questions"])


def test_distribution_and_case_level_agreement():
    assert distribution([0, 1])["p90"] == 0.9
    assert distribution([])["mean"] is None
    assert agreement([False, True], [False, True])["kappa"] == 1
    assert agreement([True, True], [True, True])["kappa"] is None
    f = fixture()
    records = []
    for provider in ["local", "hosted"]:
        for repeat in range(4):
            records.append(
                dict(
                    phase="sample",
                    provider=provider,
                    mode="bundled",
                    case="none",
                    repeat=repeat,
                    status="complete",
                    elapsed_seconds=2,
                    request=dict(questions=f["questions"]),
                    scores=dict(looping=0.8, spiraling=0.9, harmful_language=0.7),
                )
            )
    records.append(dict(records[0], status="unavailable"))
    records.append(dict(records[0], phase="warmup", elapsed_seconds=200))
    report = summarize(records, f)
    assert report["attempted"] == 9 and report["failed"] == 1
    local = next(
        r
        for r in report["probabilities"]
        if r["provider"] == "local" and r["dimension"] == "spiraling"
    )
    assert local["mean"] == 0.9  # Independent nouls must not normalize to sum one.
    assert local["attempted"] == 5 and local["n"] == 4
    matching = next(
        r
        for r in report["interrater_agreement"]
        if r["comparison"] == "local/bundled - hosted/bundled"
    )
    assert matching["n"] == 1  # Four repeats are one independently authored item.


def test_fixture_requires_all_gold_labels(tmp_path):
    f = fixture()
    del f["cases"][0]["expected"]["looping"]
    path = tmp_path / "cases.json"
    path.write_text(json.dumps(f))
    with pytest.raises(ValueError, match="every question"):
        eval_run.load_fixture(path)


@pytest.mark.asyncio
async def test_preflight_failure_is_saved_without_sample_retries(tmp_path, monkeypatch):
    f = fixture()
    path = tmp_path / "cases.json"
    path.write_text(json.dumps(f))

    async def unavailable(request, record):
        record.update(
            status="unavailable",
            error="Missing key",
            request=request,
            provider="openrouter",
        )

    monkeypatch.setattr(eval_run, "classify", unavailable)
    args = eval_run.parser().parse_args(
        [
            "--cases",
            str(path),
            "--providers",
            "hosted",
            "--output",
            str(tmp_path / "results"),
        ]
    )
    assert await eval_run.run(args) == 1
    summary = json.loads((tmp_path / "results" / "summary.json").read_text())
    assert summary["attempted"] == 0 and summary["not_run"] == 80
    records = (tmp_path / "results" / "requests.jsonl").read_text().splitlines()
    assert len(records) == 1 and json.loads(records[0])["phase"] == "warmup"


def test_all_failed_bucket_is_visible():
    f = fixture()
    row = dict(
        phase="sample",
        provider="local",
        mode="bundled",
        case="none",
        repeat=0,
        status="unavailable",
        elapsed_seconds=1,
        request=dict(questions=f["questions"]),
    )
    summary = summarize([row], f)
    assert len(summary["probabilities"]) == 3
    assert all(
        r["n"] == 0 and r["failed"] == 1 and r["mean"] is None
        for r in summary["probabilities"]
    )
    eval_run.markdown(summary)


def test_bundled_question_order_identical_across_repeats_providers():
    f = fixture()
    jobs = eval_run.plan(f, ["local", "hosted"], ["bundled", "separate"], 10, 7)
    assert all(
        j["questions"] == list(f["questions"]) for j in jobs if j["mode"] == "bundled"
    )


@pytest.mark.asyncio
async def test_request_questions_match_real_monitor_scan(monkeypatch):
    from character_lab import monitor

    captured = {}

    async def capture(request, record, **kwargs):
        captured.update(request)
        record.update(status="unavailable")

    monkeypatch.setattr(monitor, "classify", capture)
    f = fixture()
    turn = dict(role="character", text="A coherent reply.", status="complete")
    config = dict(
        monitor_model="model",
        monitor_dimensions=[
            dict(id=name, spec=value["instructions"], enabled=True)
            for name, value in f["questions"].items()
        ],
    )
    await monitor.scan(config, dict(turns=[turn]), turn)
    assert (
        eval_run.request_for(f, f["cases"][0], "model", list(f["questions"]))
        == captured
    )
