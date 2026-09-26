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
    assert turn["monitor"]["response"]["model"] == "jev-test"
    assert "test-only" not in json.dumps(turn)
    assert turn["text"] == "raw reply"


@pytest.mark.asyncio
async def test_missing_key_is_visible_without_call(monkeypatch):
    monkeypatch.delenv("OPENROUTER_API_KEY", raising=False)
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
    assert turn["monitor"]["http_status"] == 503


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
