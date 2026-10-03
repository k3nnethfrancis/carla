"""Own one cached MLX judge per Carla backend; never discover or kill other servers."""

import asyncio
import os
import platform
import signal
import tempfile
import time
from pathlib import Path

import httpx

PACKAGE = "openjev[mlx] @ git+https://github.com/razorback16/openjev@a0ddd7d928298eccef2c17153b00b5636b6d996a"
SETUP = "Run ./scripts/local-judge.sh once to install the local judge (about 17 GB), then stop it and retry."


def runtime_python():
    from .workspaces import HOME

    return HOME / "runtimes" / "openjev-a0ddd7d9-v1" / "bin" / "python"


class LocalJudgeError(RuntimeError):
    """Safe, user-facing startup failure without credentials or subprocess output."""


class LocalJudge:
    def __init__(self):
        self.process = None
        self.endpoint = None
        self.lock = asyncio.Lock()
        self.log = None
        self.failure = None
        self.retry_at = 0

    async def ensure(self):
        # Concurrent monitor/evaluation requests share this single startup.
        async with self.lock:
            if self.process is not None and self.process.returncode is None:
                return self.endpoint
            if self.failure and time.monotonic() < self.retry_at:
                raise LocalJudgeError(self.failure)
            await self.close()
            if platform.system() != "Darwin" or platform.machine() != "arm64":
                raise LocalJudgeError("Managed DiffusionGemma requires Apple Silicon.")
            python = runtime_python()
            if not python.is_file():
                raise LocalJudgeError("Local judge runtime is not prepared. " + SETUP)
            env = dict(
                os.environ,
                UV_OFFLINE="1",
                HF_HUB_OFFLINE="1",
                TRANSFORMERS_OFFLINE="1",
                CARLA_JUDGE_PARENT=str(os.getpid()),
            )
            self.log = tempfile.TemporaryFile()
            try:
                self.process = await asyncio.create_subprocess_exec(
                    str(python),
                    str(Path(__file__).with_name("local_judge_worker.py")),
                    env=env,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=self.log,
                    start_new_session=True,
                )
                async with asyncio.timeout(180):
                    line = await self.process.stdout.readline()
                    if not line.startswith(b"CARLA_JUDGE_PORT="):
                        raise LocalJudgeError(
                            "Local judge is not installed or could not start. " + SETUP
                        )
                    port = int(line.strip().split(b"=", 1)[1])
                    self.endpoint = f"http://127.0.0.1:{port}"
                    async with httpx.AsyncClient(
                        timeout=2, trust_env=False, follow_redirects=False
                    ) as client:
                        while self.process.returncode is None:
                            try:
                                response = await client.get(self.endpoint + "/health")
                                if (
                                    response.status_code == 200
                                    and response.json().get("status") == "ok"
                                ):
                                    self.failure = None
                                    return self.endpoint
                            except (httpx.HTTPError, ValueError):
                                pass
                            await asyncio.sleep(0.2)
                    raise LocalJudgeError("Local judge exited during startup. " + SETUP)
            except BaseException as exc:
                diagnostic = ""
                if isinstance(exc, (LocalJudgeError, TimeoutError)) and self.log:
                    # Startup stderr contains the actual dependency/model error.
                    # Retain only its bounded tail on failure, outside the repo;
                    # NamedTemporaryFile gives the diagnostic owner-only access.
                    self.log.seek(0, os.SEEK_END)
                    self.log.seek(max(0, self.log.tell() - 65536))
                    with tempfile.NamedTemporaryFile(
                        prefix="carla-local-judge-", suffix=".log", delete=False
                    ) as output:
                        tail = self.log.read()
                        output.write(tail)
                        if b"Network connectivity is disabled" in tail:
                            exc = LocalJudgeError(
                                "Local judge Python dependencies are missing from the offline cache. "
                                + SETUP
                            )
                        diagnostic = " Startup log: " + output.name
                await self.close()
                if isinstance(exc, TimeoutError):
                    exc = LocalJudgeError("Local judge startup timed out.")
                if isinstance(exc, LocalJudgeError):
                    self.failure = str(exc) + diagnostic
                    self.retry_at = time.monotonic() + 60
                    raise LocalJudgeError(self.failure) from None
                raise

    async def close(self):
        process, self.process = self.process, None
        self.endpoint = None
        if process is not None and process.returncode is None:
            # The session is ours, including uv's Python child; never signal a discovered PID.
            try:
                os.killpg(process.pid, signal.SIGTERM)
                await asyncio.wait_for(process.wait(), 5)
            except ProcessLookupError:
                pass
            except TimeoutError:
                os.killpg(process.pid, signal.SIGKILL)
                await process.wait()
        if self.log is not None:
            self.log.close()
            self.log = None


managed = LocalJudge()
