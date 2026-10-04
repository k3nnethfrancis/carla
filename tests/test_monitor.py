import json

import httpx
import pytest

from character_lab import monitor, simulator


@pytest.mark.asyncio
async def test_monitor_preserves_exact_history_and_structured_scores(monkeypatch):
    monkeypatch.setenv("OPENROUTER_API_KEY", "test-only")
    requests = []

    def handle(request):
        assert str(request.url) == "https://openrouter.ai/api/v1/systemone"
        payload = json.loads(request.content)
        requests.append(payload)
        return httpx.Response(
            200,
            json={
                "model": "jev-test",
                "answers": {
                    name: {"type": "noul", "noul": 0.99}
                    for name in payload["questions"]
                },
                "usage": {"input_tokens": 20, "output_tokens": 3},
            },
        )

    real = httpx.AsyncClient
    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: real(transport=httpx.MockTransport(handle), **kw),
    )
    config = simulator.defaults("base")
    config.pop("monitor_call_mode")  # Existing workspaces default to separate too.
    convo = {
        "turns": [
            {"role": "user", "text": "hello"},
            {"role": "character", "text": "raw reply"},
        ]
    }
    turn = convo["turns"][-1]
    await monitor.scan(config, convo, turn)
    assert requests[0]["state"]["history"] == [
        {"role": "user", "text": "hello"},
        {"role": "character", "text": "raw reply"},
    ]
    assert turn["monitor"]["scores"]["looping"] == 0.99
    assert len(requests) == 3
    assert all(len(r["questions"]) == 1 for r in requests)
    assert all(r["state"] == requests[0]["state"] for r in requests)
    assert turn["monitor"]["calls"][0]["response"]["model"] == "jev-test"
    assert "response" not in turn["monitor"]
    assert "test-only" not in json.dumps(turn)
    assert turn["text"] == "raw reply"


@pytest.mark.asyncio
async def test_missing_key_is_visible_without_call(monkeypatch):
    monkeypatch.setattr(monitor, "openrouter_key", lambda: ("", "none"))
    turn = {"role": "character", "text": "text"}
    await monitor.scan(simulator.defaults("base"), {"turns": [turn]}, turn)
    assert turn["monitor"]["status"] == "unavailable"


@pytest.mark.asyncio
async def test_http_error_is_advisory(monkeypatch):
    monkeypatch.setenv("OPENROUTER_API_KEY", "test-only")
    real = httpx.AsyncClient
    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: real(
            transport=httpx.MockTransport(lambda r: httpx.Response(503)), **kw
        ),
    )
    turn = {"role": "character", "text": "text"}
    await monitor.scan(simulator.defaults("base"), {"turns": [turn]}, turn)
    assert turn["monitor"]["status"] == "unavailable"
    assert all(c["http_status"] == 503 for c in turn["monitor"]["calls"])


def test_dimension_decision_rules_and_default_protection():
    config = {"monitor_dimensions": monitor.dimensions({})}
    ds = config["monitor_dimensions"]
    ds[1]["enabled"] = False
    ds[2].update(decision="threshold", threshold=0.8, action="stop")
    assert (
        monitor.detections(
            config, {"looping": 0.5, "spiraling": 1, "harmful_language": 0.79}
        )
        == []
    )
    hits = monitor.detections(config, {"looping": 0.51, "harmful_language": 0.8})
    assert [h["action"] for h in hits] == ["warn", "stop"]
    with pytest.raises(ValueError, match="disabled, not deleted"):
        monitor.validate_dimensions(ds[1:])
    ds[0]["threshold"] = float("nan")
    with pytest.raises(ValueError, match="threshold"):
        monitor.validate_dimensions(ds)


@pytest.mark.asyncio
async def test_disabled_dimensions_do_not_call_provider(monkeypatch):
    config = {
        "monitor_model": "jev-latest",
        "monitor_dimensions": monitor.dimensions({}),
    }
    for d in config["monitor_dimensions"]:
        d["enabled"] = False

    def no_client(**kw):
        raise AssertionError("Disabled policy must not call provider")

    monkeypatch.setattr(httpx, "AsyncClient", no_client)
    turn = {"role": "character", "text": "raw"}
    await monitor.scan(config, {"turns": [turn]}, turn)
    assert turn["monitor"]["detections"] == []


@pytest.mark.parametrize(
    "address",
    [
        "https://example.com",
        "http://10.0.0.1:8080",
        "http://127.0.0.1:8080@evil.com",
        "http://localhost:8080/path",
        "http://localhost:8080?key=secret",
        "http://localhost:8080#frag",
        "http://localhost",
        None,
    ],
)
def test_local_endpoint_rejects_non_loopback_or_ambiguous_addresses(address):
    with pytest.raises(ValueError, match="loopback"):
        monitor.local_url(address)


async def test_local_monitor_no_key_no_proxy_and_preserves_evidence(monkeypatch):
    monkeypatch.setenv("OPENROUTER_API_KEY", "must-never-leak")

    def handle(request):
        assert str(request.url) == "http://127.0.0.1:8080/v1/systemone"
        assert "authorization" not in request.headers
        payload = json.loads(request.content)
        assert payload["model"] == "openjev-latest"
        assert payload["state"]["history"][-1]["text"] == "Again. Again."
        return httpx.Response(
            200,
            json={
                "model": "openjev-0.1",
                "answers": {q: {"noul": 0.9} for q in payload["questions"]},
            },
        )

    real = httpx.AsyncClient

    def client(**kw):
        assert kw["trust_env"] is False and kw["follow_redirects"] is False
        return real(transport=httpx.MockTransport(handle), **kw)

    monkeypatch.setattr(httpx, "AsyncClient", client)
    config = simulator.defaults("base") | {
        "monitor_mode": "diffusion",
        "monitor_local_url": "http://127.0.0.1:8080",
    }
    turn = {"role": "character", "text": "Again. Again."}
    await monitor.scan(config, {"turns": [turn]}, turn)
    record = turn["monitor"]
    assert record["status"] == "complete"
    assert record["provider"] == "openjev"
    assert len(record["detections"]) == 3
    assert all(c["response"]["model"] == "openjev-0.1" for c in record["calls"])
    assert "must-never-leak" not in json.dumps(record)


@pytest.mark.parametrize(
    "failure", ["offline", "redirect", "overflow", "bad_score", "missing_score"]
)
async def test_local_classifier_failures_never_fallback_or_detect(monkeypatch, failure):
    requests = []

    def handle(request):
        requests.append(request)
        if failure == "offline":
            raise httpx.ConnectError("offline")
        if failure == "redirect":
            return httpx.Response(302, headers={"Location": "https://example.com"})
        if failure == "overflow":
            return httpx.Response(400)
        return httpx.Response(
            200,
            json={
                "answers": {"looping": {"noul": True}} if failure == "bad_score" else {}
            },
        )

    real = httpx.AsyncClient
    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: real(transport=httpx.MockTransport(handle), **kw),
    )
    config = simulator.defaults("base") | {
        "monitor_mode": "diffusion",
        "monitor_local_url": "http://127.0.0.1:8080",
    }
    turn = {"role": "character", "text": "text"}
    await monitor.scan(config, {"turns": [turn]}, turn)
    assert len(requests) == 3
    assert turn["monitor"]["status"] == "unavailable"
    assert not turn["monitor"].get("detections")
    assert not turn["monitor"].get("scores")


async def test_invalid_explicit_local_endpoint_never_selects_hosted(monkeypatch):
    def forbidden():
        raise AssertionError("Local request accessed hosted credentials")

    monkeypatch.setattr(monitor, "openrouter_key", forbidden)
    record = {}
    await monitor.classify({"questions": {"passes": {}}}, record, endpoint=None)
    assert record["status"] == "unavailable" and record["provider"] == "openjev"
    assert "endpoint" not in record


@pytest.mark.parametrize("mode,count", [("separate", 2), ("bundled", 1)])
async def test_call_modes_skip_disabled_and_freeze_input(monkeypatch, mode, count):
    config = simulator.defaults("base") | {"monitor_call_mode": mode}
    config["monitor_dimensions"][1]["enabled"] = False
    turn = {"role": "character", "text": "original", "status": "generating"}
    payloads = []

    async def classify(request, record):
        payloads.append(request)
        turn["text"] += " streaming"
        record.update(
            status="complete",
            request=request,
            response={"answers": {k: {"noul": 0.7} for k in request["questions"]}},
            scores={k: 0.7 for k in request["questions"]},
            elapsed_seconds=0.1,
        )

    monkeypatch.setattr(monitor, "classify", classify)
    await monitor.scan(config, {"turns": [turn]}, turn)
    record = turn["monitor"]
    assert len(payloads) == count
    assert all(r["state"]["history"][-1]["text"] == "original" for r in payloads)
    assert all(not r["state"]["latest_message_complete"] for r in payloads)
    assert set(record["scores"]) == {"looping", "harmful_language"}
    assert len(record["calls"]) == count
    assert ("response" in record) == (mode == "bundled")


async def test_separate_partial_failure_preserves_success_not_zero(monkeypatch):
    config = simulator.defaults("base")
    for d in config["monitor_dimensions"]:
        d.update(action="stop", decision="threshold", threshold=0)

    async def classify(request, record):
        name = next(iter(request["questions"]))
        record["request"] = request
        if name == "spiraling":
            record.update(status="unavailable", error="HTTPStatusError")
        else:
            record.update(status="complete", scores={name: 0.9})

    monkeypatch.setattr(monitor, "classify", classify)
    turn = {"role": "character", "text": "text"}
    await monitor.scan(config, {"turns": [turn]}, turn)
    record = turn["monitor"]
    assert record["status"] == "partial"
    assert set(record["scores"]) == {"looping", "harmful_language"}
    assert [d["id"] for d in record["detections"]] == ["looping", "harmful_language"]
    assert record["calls"][1]["error"] == "HTTPStatusError"


async def test_cancelled_check_retains_attempted_requests(monkeypatch):
    import asyncio

    async def classify(request, record):
        record["request"] = request
        record["elapsed_seconds"] = 0.2
        raise asyncio.CancelledError

    monkeypatch.setattr(monitor, "classify", classify)
    turn = {"role": "character", "text": "text"}
    with pytest.raises(asyncio.CancelledError):
        await monitor.scan(simulator.defaults("base"), {"turns": [turn]}, turn)
    record = turn["monitor"]
    assert record["status"] == "cancelled"
    assert len(record["calls"]) == 1
    assert record["calls"][0]["status"] == "cancelled"
    assert record["calls"][0]["request"]["questions"].keys() == {"looping"}


def test_call_mode_default_and_validation():
    from types import SimpleNamespace

    project = SimpleNamespace(data={"models": [{"alias": "base"}]})
    config = simulator.configuration(project, "base")
    assert config["monitor_call_mode"] == "separate"
    config["monitor_call_mode"] = "invalid"
    with pytest.raises(ValueError, match="Separate or Bundled"):
        simulator.validate(config, project, lambda *_: None)


async def test_full_calls_stay_in_run_not_stream_view(monkeypatch):
    async def classify(request, record):
        record.update(
            status="complete",
            request=request,
            response={"raw": "evidence"},
            scores={name: 0.1 for name in request["questions"]},
        )

    monkeypatch.setattr(monitor, "classify", classify)
    turn = {"role": "character", "text": "text"}
    conversation = {"turns": [turn]}
    await monitor.scan(simulator.defaults("base"), conversation, turn)
    turn["monitor_checks"] = [turn["monitor"]]
    run = {"id": "test", "conversations": [conversation]}
    persisted = json.loads(json.dumps(run))
    assert len(persisted["conversations"][0]["turns"][0]["monitor"]["calls"]) == 3
    visible = simulator.view(run)["conversations"][0]["turns"][0]
    assert "calls" not in visible["monitor"]
    assert "calls" not in visible["monitor_checks"][0]
    assert visible["monitor"]["scores"] == turn["monitor"]["scores"]
    assert run["conversations"][0]["turns"][0]["monitor"]["calls"][0]["response"] == {
        "raw": "evidence"
    }
