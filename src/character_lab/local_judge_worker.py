"""Executed inside the pinned optional uv environment, never Carla's environment."""

import os
import signal
import socket
import threading
import time


def main():
    from huggingface_hub import snapshot_download

    model = snapshot_download(
        "mlx-community/diffusiongemma-26B-A4B-it-4bit",
        revision="a7a81407613811e8ba63af92ac0d852b809e191f",
        local_files_only=True,
    )
    os.environ.update(
        OPENJEV_BACKEND="mlx",
        OPENJEV_HOST="127.0.0.1",
        OPENJEV_MLX_MODEL=model,
        OPENJEV_MODEL_ROUTES="",
        OPENJEV_API_KEY="",
        HF_HUB_OFFLINE="1",
        TRANSFORMERS_OFFLINE="1",
    )
    parent = int(os.environ["CARLA_JUDGE_PARENT"])

    def watch_parent():
        # Also clean up if Carla is killed before its normal async shutdown.
        while True:
            time.sleep(1)
            try:
                os.kill(parent, 0)
            except ProcessLookupError:
                os.killpg(os.getpgrp(), signal.SIGTERM)
                return

    threading.Thread(target=watch_parent, daemon=True).start()
    import uvicorn
    from openjev.api import create_app

    # Keep the bound socket through startup: no find-free-port / bind race.
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        listener.listen(128)
        print(f"CARLA_JUDGE_PORT={listener.getsockname()[1]}", flush=True)
        server = uvicorn.Server(uvicorn.Config(create_app(), log_level="warning"))
        server.run(sockets=[listener])


if __name__ == "__main__":
    main()
