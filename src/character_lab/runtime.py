"""Local raw completion only: no chat template or hidden instructions."""

from __future__ import annotations

import asyncio
import json
import shutil
import subprocess
from pathlib import Path

import httpx

from .scheduling import MAX_WORKERS, Admission

DEFAULT_MODEL = {
    "name": "Configure a base model",
    "alias": "character-lab-base",
    "path": "",
    "url": "http://127.0.0.1:18986",
    "port": 18986,
    "kind": "base",
    "context": 8192,
}


class Runtime:
    def __init__(self, folder, model=None):
        self.model = model or DEFAULT_MODEL.copy()
        self.folder = folder
        self.process = None
        self.sleep_guard = None
        self._loading = asyncio.Lock()
        self.admission = Admission()
        self.on_schedule = None

    async def ensure(self):
        # Concurrent requests share one startup and one model process.
        async with self._loading:
            await self._ensure()

    async def _ensure(self):
        async with httpx.AsyncClient(timeout=3, trust_env=False) as client:
            try:
                r = await client.get(self.model["url"] + "/health")
                if r.status_code == 200:
                    models = (await client.get(self.model["url"] + "/v1/models")).json()
                    if self.model["alias"] not in [m["id"] for m in models["data"]]:
                        raise ValueError(
                            "A different model owns this endpoint. Stop it or choose another endpoint."
                        )
                    return
            except httpx.RequestError:
                pass
        path = Path(self.model["path"])
        if not path.is_file():
            raise ValueError(
                "Base model is not downloaded. See README for setup; source browsing works offline."
            )
        exe = shutil.which("llama-server")
        if not exe:
            raise ValueError("llama-server is not on PATH.")
        if not self.process or self.process.poll() is not None:
            with (self.folder / "model-server.log").open("ab") as log:
                self.process = subprocess.Popen(
                    [
                        exe,
                        "--model",
                        str(path),
                        "--host",
                        "127.0.0.1",
                        "--port",
                        str(self.model["port"]),
                        "--alias",
                        self.model["alias"],
                        "--ctx-size",
                        str(self.model["context"]),
                        "--parallel",
                        str(MAX_WORKERS),
                        "--kv-unified",
                        "--cont-batching",
                        "--no-cache-idle-slots",
                        "--cache-ram",
                        "0",
                        "--n-gpu-layers",
                        str(self.model.get("gpu_layers", 99)),
                        "--no-context-shift",
                    ],
                    stdout=log,
                    stderr=log,
                )
            if shutil.which("caffeinate"):
                self.sleep_guard = subprocess.Popen(
                    ["caffeinate", "-i", "-w", str(self.process.pid)]
                )
        for _ in range(120):
            if self.process.poll() is not None:
                raise ValueError(
                    "Model server exited. Details are saved in model-server.log."
                )
            await asyncio.sleep(1)
            async with httpx.AsyncClient(timeout=3, trust_env=False) as client:
                try:
                    if (
                        await client.get(self.model["url"] + "/health")
                    ).status_code == 200:
                        return
                except httpx.RequestError:
                    pass
        raise ValueError("Model loading timed out; see model-server.log.")

    async def context(self, client):
        """Read the loaded capacity; a configured zero means native, not zero tokens."""
        response = await client.get(self.model["url"] + "/props")
        response.raise_for_status()
        props = response.json()
        capacity = props.get("default_generation_settings", {}).get(
            "n_ctx", self.model["context"]
        )
        if type(capacity) is not int or capacity < 1:
            raise ValueError("Model server did not report a usable context capacity")
        return capacity, props

    async def stream(self, prompt, settings, trace):
        await self.ensure()
        async with httpx.AsyncClient(
            timeout=httpx.Timeout(180, connect=10), trust_env=False
        ) as client:
            tokenized = await client.post(
                self.model["url"] + "/tokenize",
                json={"content": prompt, "add_special": True},
            )
            tokenized.raise_for_status()
            count = len(tokenized.json()["tokens"])
            capacity, props = await self.context(client)
            trace["prompt_tokens"] = count
            trace["context_capacity"] = capacity
            output = (
                capacity - count
                if settings["n_predict"] == -1
                else settings["n_predict"]
            )
            requested_max = output
            if output < 1 or count + requested_max > capacity:
                raise ValueError(
                    f"Input needs {count} tokens plus {requested_max} requested output tokens; context is {capacity}. Select less text. Nothing was truncated."
                )
            request = {
                "prompt": prompt,
                "n_predict": output,
                "temperature": settings["temperature"],
                "top_p": settings["top_p"],
                "top_k": 0,
                "min_p": 0,
                "repeat_penalty": 1.0,
                "seed": settings["seed"],
                "stream": True,
                "cache_prompt": False,
            }
            if "stop" in settings:
                request["stop"] = settings["stop"]
            trace["request"] = request
            trace["model"] = self.model.copy()
            trace["events"] = []
            slots = min(MAX_WORKERS, props.get("total_slots", 1)) if self.process else 1
            async with self.admission.reserve(
                count + output, capacity, slots, trace, self.on_schedule
            ):
                trace["stream_status"] = "streaming"
                try:
                    async with client.stream(
                        "POST", self.model["url"] + "/completion", json=request
                    ) as response:
                        response.raise_for_status()
                        async for line in response.aiter_lines():
                            if not line.startswith("data: "):
                                continue
                            raw = line[6:]
                            if raw == "[DONE]":
                                trace["stream_status"] = "complete"
                                trace["terminal"] = "[DONE]"
                                return
                            event = json.loads(raw)
                            trace["events"].append(event)
                            if "error" in event:
                                raise ValueError(str(event["error"]))
                            if type(event.get("tokens_predicted")) is int:
                                trace["generated_tokens"] = event["tokens_predicted"]
                            if event.get("stop") is True:
                                trace["stream_status"] = "complete"
                                trace["terminal"] = "stop"
                            yield event.get("content", "")
                            if event.get("stop") is True:
                                return
                        trace["stream_status"] = "interrupted"
                        raise ValueError(
                            "Model stream ended without a completion event; partial output was preserved"
                        )
                except (asyncio.CancelledError, GeneratorExit):
                    if trace["stream_status"] != "complete":
                        trace["stream_status"] = "cancelled"
                    raise
                except Exception:
                    if trace["stream_status"] != "interrupted":
                        trace["stream_status"] = "failed"
                    raise

    def close(self):
        if self.process and self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait(timeout=5)
        if self.sleep_guard and self.sleep_guard.poll() is None:
            self.sleep_guard.terminate()

    async def judge(self, messages, trace):
        """Separate instruct-model call; never used for the base-model document."""
        await self.ensure()
        if self.process is None:
            raise ValueError(
                "Policy server is externally managed; stop it so Carla can own sequential model switching."
            )
        request = {
            "model": self.model["alias"],
            "messages": messages,
            "temperature": 0,
            "max_tokens": 1536,
            "stream": False,
            "response_format": {"type": "json_object"},
            "chat_template_kwargs": {"enable_thinking": False},
        }
        trace.update(model=self.model.copy(), request=request)
        async with httpx.AsyncClient(timeout=180, trust_env=False) as client:
            template = await client.post(
                self.model["url"] + "/apply-template",
                json={
                    "messages": messages,
                    "chat_template_kwargs": {"enable_thinking": False},
                },
            )
            template.raise_for_status()
            rendered = template.json()["prompt"]
            trace["rendered_prompt"] = rendered
            tokens = await client.post(
                self.model["url"] + "/tokenize",
                json={"content": rendered, "add_special": True},
            )
            tokens.raise_for_status()
            trace["prompt_tokens"] = len(tokens.json()["tokens"])
            capacity, _ = await self.context(client)
            trace["context_capacity"] = capacity
            if trace["prompt_tokens"] + request["max_tokens"] > capacity:
                raise ValueError(
                    "Policy context overflow; reduce branch count or length. Nothing was truncated."
                )
            response = await client.post(
                self.model["url"] + "/v1/chat/completions", json=request
            )
            trace["http_status"] = response.status_code
            trace["raw_response"] = response.text
            response.raise_for_status()
            result = response.json()
            trace["response"] = result
            if result["choices"][0]["finish_reason"] != "stop":
                raise ValueError("Policy response did not finish; no branch selected")
            return json.loads(result["choices"][0]["message"]["content"])
