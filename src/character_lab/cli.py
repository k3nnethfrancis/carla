"""Installed Python entry point delegates terminal ownership to the Go binary."""

import os
import sys
from pathlib import Path


def main():
    binary = (
        Path(os.environ["CARLA_BINARY"]).expanduser()
        if os.environ.get("CARLA_BINARY")
        else Path(__file__).resolve().parents[2] / "bin/carla"
    )
    if not binary.is_file():
        raise SystemExit("Build the TUI first: cd tui && go build -o ../bin/carla .")
    args = sys.argv[1:]
    os.environ["CARLA_PYTHON"] = sys.executable
    os.execv(str(binary), [str(binary), *args])
