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


@pytest.fixture
def runtime_server(tmp_path, monkeypatch):
    """Exercise the real HTTP stream parser without starting a model server."""
    real_client = httpx.AsyncClient

    def install(completion, capacity=4096):
        requests = []

        def handle(request):
            requests.append(request.url.path)
            if request.url.path == "/props":
                return httpx.Response(
                    200, json={"default_generation_settings": {"n_ctx": capacity}}
                )
            if request.url.path == "/tokenize":
                return httpx.Response(200, json={"tokens": [1, 2, 3, 4]})
            if request.url.path == "/apply-template":
                return httpx.Response(200, json={"prompt": "rendered policy"})
            if request.url.path == "/v1/chat/completions":
                return httpx.Response(
                    200,
                    json={
                        "choices": [
                            {
                                "finish_reason": "stop",
                                "message": {"content": '{"selected": 1}'},
                            }
                        ]
                    },
                )
            assert request.url.path == "/completion"
            return completion()

        monkeypatch.setattr(
            httpx,
            "AsyncClient",
            lambda **kwargs: real_client(
                transport=httpx.MockTransport(handle), **kwargs
            ),
        )
        runtime = Runtime(tmp_path)
        runtime.model = {**runtime.model, "context": 0}

        async def ready():
            pass

        monkeypatch.setattr(runtime, "ensure", ready)
        return runtime, requests

    return install


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "ending,status",
    [
        ('data: {"content":" tail","stop":true}\n\n', "complete"),
        ("data: [DONE]\n\n", "complete"),
        ("", "interrupted"),
        ('data: {"error":"provider failed"}\n\n', "failed"),
    ],
)
async def test_stream_requires_terminal_and_keeps_evidence(
    runtime_server, ending, status
):
    runtime, _ = runtime_server(
        lambda: httpx.Response(
            200, text='data: {"content":"prefix","stop":false}\n\n' + ending
        )
    )
    trace, chunks = {}, []
    error = None
    try:
        async for chunk in runtime.stream(
            "prompt", dict(n_predict=10, temperature=1, top_p=1, seed=1), trace
        ):
            chunks.append(chunk)
    except ValueError as exc:
        error = exc
    assert bool(error) == (status != "complete")
    assert chunks[0] == "prefix"
    assert trace["events"][0]["content"] == "prefix"
    assert trace["stream_status"] == status
    assert runtime.admission.active == runtime.admission.tokens == 0
    if status == "failed":
        assert trace["events"][-1] == {"error": "provider failed"}
    if "tail" in ending:
        assert "".join(chunks) == "prefix tail"


@pytest.mark.asyncio
@pytest.mark.parametrize("capacity,succeeds", [(4096, True), (1539, False), (0, False)])
async def test_policy_uses_loaded_native_context(runtime_server, capacity, succeeds):
    runtime, requests = runtime_server(lambda: None, capacity)
    runtime.process = object()  # Selector ownership remains managed.
    trace = {}
    if succeeds:
        assert await runtime.judge([{"role": "user", "content": "select"}], trace) == {
            "selected": 1
        }
        assert trace["context_capacity"] == capacity
        assert trace["rendered_prompt"] == "rendered policy"
    else:
        with pytest.raises(ValueError):
            await runtime.judge([], trace)
        assert "/v1/chat/completions" not in requests


@pytest.mark.asyncio
async def test_cancel_stalled_http_stream_releases_capacity(runtime_server):
    import asyncio

    closed, reading = asyncio.Event(), asyncio.Event()

    class Stalled(httpx.AsyncByteStream):
        async def __aiter__(self):
            yield b'data: {"content":"prefix","stop":false}\n\n'
            reading.set()
            await asyncio.Event().wait()

        async def aclose(self):
            closed.set()

    runtime, _ = runtime_server(lambda: httpx.Response(200, stream=Stalled()))
    trace = {}
    stream = runtime.stream(
        "prompt", dict(n_predict=10, temperature=1, top_p=1, seed=1), trace
    )
    assert await anext(stream) == "prefix"
    read = asyncio.create_task(anext(stream))
    await asyncio.wait_for(reading.wait(), 1)
    read.cancel()
    with pytest.raises(asyncio.CancelledError):
        await read
    await stream.aclose()
    assert closed.is_set()
    assert trace["stream_status"] == "cancelled"
    assert runtime.admission.active == runtime.admission.tokens == 0


@pytest.mark.asyncio
async def test_truncated_simulation_preserves_partial_and_does_not_advance(
    runtime_server, tmp_path, monkeypatch
):
    from character_lab import simulator
    from character_lab.domain import Project

    runtime, requests = runtime_server(
        lambda: httpx.Response(
            200, text='data: {"content":"partial reply","stop":false}\n\n'
        )
    )
    monkeypatch.setattr(runtime, "close", lambda: None)
    project = Project(tmp_path)
    project.data["models"] = [runtime.model]
    source = project.add("Paths.", kind="source")
    source["kept"] = True
    config = simulator.defaults(runtime.model["alias"])
    config.update(documents=[source["id"]], turns=2)

    async def emit(*args):
        pass

    run = await simulator.generate(project, config, lambda *args: runtime, emit)
    assert run["status"] == "failed"
    assert "without a completion event" in run["error"]
    assert requests.count("/completion") == 1
    saved = Project(tmp_path).data["simulation_runs"][0]
    turns = saved["conversations"][0]["turns"]
    assert len(turns) == 2
    assert turns[-1]["text"] == "partial reply"
    assert turns[-1]["status"] == "failed"
    assert turns[-1]["trace"]["stream_status"] == "interrupted"
