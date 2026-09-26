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
    setup = "--setup-model" in args
    args = [arg for arg in args if arg != "--setup-model"]
    if not any(arg in {"--help", "-h"} for arg in args):
        from .model_setup import needs_setup, run

        try:
            if setup or (sys.stdin.isatty() and needs_setup(args)):
                if not sys.stdin.isatty():
                    raise SystemExit("Model setup requires an interactive terminal")
                run()
        except (KeyboardInterrupt, EOFError):
            raise SystemExit("Model setup cancelled; run carla to try again") from None
        except Exception as exc:
            raise SystemExit(f"Model setup failed: {exc}") from None
    os.environ["CARLA_PYTHON"] = sys.executable
    os.execv(str(binary), [str(binary), *args])
