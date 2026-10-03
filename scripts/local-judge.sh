#!/bin/sh
# Pinned, optional Apple Silicon classifier. Generation stays on llama.cpp.
# uv keeps this dependency environment separate from Carla's .venv.
set -eu
if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
    echo 'This launcher requires Apple Silicon. See docs/local-judge.md for other hosts.' >&2
    exit 1
fi
runtime="${CARLA_DATA_DIR:-$HOME/.local/share/character-lab}/runtimes/openjev-a0ddd7d9-v1"
if [ ! -f "$runtime/ready" ]; then
    uv venv --python 3.12 "$runtime"
    uv pip install --python "$runtime/bin/python" 'openjev[mlx] @ git+https://github.com/razorback16/openjev@a0ddd7d928298eccef2c17153b00b5636b6d996a'
    uv pip freeze --python "$runtime/bin/python" > "$runtime/installed.txt"
    touch "$runtime/ready"
fi
exec "$runtime/bin/python" - "$@" <<'PY'
import os
import runpy
import sys

from huggingface_hub import snapshot_download

# Download once into the normal HF cache; no second model copy in Carla.
model = snapshot_download(
    "mlx-community/diffusiongemma-26B-A4B-it-4bit",
    revision="a7a81407613811e8ba63af92ac0d852b809e191f",
    local_files_only=os.environ.get("HF_HUB_OFFLINE") == "1",
)
if "--setup-only" in sys.argv:
    print("Local judge runtime and checkpoint ready")
    raise SystemExit(0)
# Lock this companion to local inference, regardless of inherited OpenJev routing.
os.environ.update(
    OPENJEV_BACKEND="mlx",
    OPENJEV_HOST="127.0.0.1",
    OPENJEV_MLX_MODEL=model,
    OPENJEV_MODEL_ROUTES="",
    OPENJEV_API_KEY="",
    HF_HUB_OFFLINE="1",
    TRANSFORMERS_OFFLINE="1",
)
runpy.run_module("openjev", run_name="__main__")
PY
