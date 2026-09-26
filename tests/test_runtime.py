import json

import httpx
import pytest

from character_lab.runtime import Runtime


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "requested,expected", [(-1, 32764), (12000, 12000), (32765, None)]
)
async def test_output_budget_uses_actual_loaded_context(
    tmp_path, monkeypatch, requested, expected
):
    requests = []

    def handle(request):
        if request.url.path == "/tokenize":
            return httpx.Response(200, json={"tokens": [1, 2, 3, 4]})
        if request.url.path == "/props":
            return httpx.Response(
                200, json={"default_generation_settings": {"n_ctx": 32768}}
            )
        requests.append(json.loads(request.content))
        return httpx.Response(200, text='data: {"content":"text","stop":true}\n\n')

    real_client = httpx.AsyncClient
    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kwargs: real_client(transport=httpx.MockTransport(handle), **kwargs),
    )
    runtime = Runtime(tmp_path)

    async def ready():
        pass

    monkeypatch.setattr(runtime, "ensure", ready)
    settings = dict(n_predict=requested, temperature=3, top_p=1, seed=1)
    trace = {}
    if expected is None:
        with pytest.raises(ValueError, match="Nothing was truncated"):
            _ = [text async for text in runtime.stream("test", settings, trace)]
        assert not requests
    else:
        _ = [text async for text in runtime.stream("test", settings, trace)]
        assert requests[0]["n_predict"] == expected
        assert trace["context_capacity"] == 32768
