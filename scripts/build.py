"""Build the frontend using the app version declared by the Python package."""

import subprocess
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def main():
    version = tomllib.loads((ROOT / "pyproject.toml").read_text())["project"]["version"]
    # Source archives have no Git metadata; version alone still identifies a release.
    revision = ""
    if (ROOT / ".git").exists():
        revision = subprocess.check_output(
            ["git", "describe", "--always", "--dirty"], cwd=ROOT, text=True
        ).strip()
    subprocess.run(
        [
            "go",
            "build",
            "-ldflags",
            f"-X main.appVersion={version} -X main.appRevision={revision}",
            "-o",
            str(ROOT / "bin" / "carla"),
            ".",
        ],
        cwd=ROOT / "tui",
        check=True,
    )


if __name__ == "__main__":
    main()
