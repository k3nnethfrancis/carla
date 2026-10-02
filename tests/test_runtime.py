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
            await runtime.preflight("test", settings)
        assert not requests
    else:
        budget = await runtime.preflight("test", settings)
        assert budget == {"prompt_tokens": 4, "context_capacity": 32768}
        assert not requests
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


@pytest.mark.asyncio
@pytest.mark.parametrize("outcome", ["ready", "timeout", "wrong_model"])
async def test_managed_server_startup_and_cleanup(tmp_path, monkeypatch, outcome):
    """A failed startup must not strand model weights or the sleep guard."""
    import asyncio

    from character_lab import runtime as runtime_module

    model_path = tmp_path / "synthetic.gguf"
    model_path.touch()
    model = {
        **runtime_module.DEFAULT_MODEL,
        "path": str(model_path),
        "alias": "synthetic",
        "context": 0,
    }
    spawned = []

    class Process:
        def __init__(self, command):
            self.command = command
            self.pid = 1234
            self.terminated = False
            self.waited = False
            spawned.append(self)

        def poll(self):
            return 0 if self.terminated else None

        def terminate(self):
            self.terminated = True

        def wait(self, timeout=None):
            self.waited = True
            return 0

        def kill(self):
            self.terminated = True

    monkeypatch.setattr(
        runtime_module.subprocess, "Popen", lambda cmd, **kw: Process(cmd)
    )
    monkeypatch.setattr(
        runtime_module.shutil,
        "which",
        lambda name: "/synthetic/" + name,
    )
    real_client = httpx.AsyncClient

    def handle(request):
        if request.url.path == "/health":
            running = any(p.command[0].endswith("llama-server") for p in spawned)
            return httpx.Response(200 if outcome != "timeout" and running else 503)
        assert request.url.path == "/v1/models"
        alias = "other" if outcome == "wrong_model" else "synthetic"
        return httpx.Response(200, json={"data": [{"id": alias}]})

    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kwargs: real_client(transport=httpx.MockTransport(handle), **kwargs),
    )

    async def no_delay(_seconds):
        pass

    monkeypatch.setattr(asyncio, "sleep", no_delay)
    runtime = Runtime(tmp_path, model)
    if outcome == "ready":
        await asyncio.gather(runtime.ensure(), runtime.ensure())
    else:
        message = "timed out" if outcome == "timeout" else "different model"
        with pytest.raises(ValueError, match=message):
            await runtime.ensure()
    assert [p.command[0] for p in spawned].count("/synthetic/llama-server") == 1
    assert [p.command[0] for p in spawned].count("caffeinate") == 1
    assert "--ctx-size" in spawned[0].command
    assert spawned[0].command[spawned[0].command.index("--ctx-size") + 1] == "0"
    if outcome == "ready":
        runtime.close()
    assert all(p.terminated for p in spawned)
    assert all(p.waited for p in spawned)


@pytest.mark.asyncio
async def test_native_context_does_not_silently_shrink(runtime_server, monkeypatch):
    from character_lab import runtime as module

    runtime, requests = runtime_server(lambda: httpx.Response(200), capacity=8192)
    runtime.model["context"] = 0
    monkeypatch.setattr(module, "native_context", lambda model: 32768)
    with pytest.raises(ValueError, match="server loaded 8192"):
        await runtime.preflight("prompt", dict(n_predict=512))
    assert "/completion" not in requests


@pytest.mark.asyncio
@pytest.mark.parametrize("finish", ["stop", "length", None])
async def test_judge_stream_reports_real_deltas_and_retains_partial_output(
    tmp_path, monkeypatch, finish
):
    chunks = ['{"passed":', 'true,"reason":"clear","evidence":"text"}']
    received = []
    real_client = httpx.AsyncClient

    def handle(request):
        if request.url.path == "/apply-template":
            return httpx.Response(200, json={"prompt": "text"})
        if request.url.path == "/tokenize":
            return httpx.Response(200, json={"tokens": [1]})
        if request.url.path == "/props":
            return httpx.Response(
                200, json={"default_generation_settings": {"n_ctx": 4096}}
            )
        assert json.loads(request.content)["stream"] is True
        events = [
            {"choices": [{"delta": {"content": t}, "finish_reason": None}]}
            for t in chunks
        ]
        if finish:
            events.append({"choices": [{"delta": {}, "finish_reason": finish}]})
        return httpx.Response(
            200,
            text="".join("data: " + json.dumps(e) + "\n\n" for e in events)
            + "data: [DONE]\n\n",
        )

    monkeypatch.setattr(
        httpx,
        "AsyncClient",
        lambda **kw: real_client(transport=httpx.MockTransport(handle), **kw),
    )
    runtime = Runtime(tmp_path)
    runtime.process = object()

    async def ready():
        pass

    async def progress(text):
        received.append(text)

    monkeypatch.setattr(runtime, "ensure", ready)
    runtime.on_judge_token = progress
    trace = {}
    if finish == "stop":
        result = await runtime.judge([{"role": "user", "content": "text"}], trace)
        assert result["passed"] is True
    else:
        with pytest.raises(ValueError, match="partial output retained"):
            await runtime.judge([], trace)
    assert received == chunks
    assert trace["raw_response"] == "".join(chunks)
    assert len(trace["response_chunks"]) >= 2
