"""Install a user-local launcher after Make has prepared the checkout."""

import os
import shlex
from pathlib import Path

root = Path(__file__).resolve().parents[1]
entry = root / ".venv/bin/carla"
folder = Path(os.environ.get("CARLA_BIN_DIR", Path.home() / ".local/bin")).expanduser()
launcher = folder / "carla"
marker = "# Carla checkout launcher"
content = f'#!/bin/sh\n{marker}\nexec {shlex.quote(str(entry))} "$@"\n'

# Replace our own launcher, but never overwrite another program named carla.
if launcher.exists() or launcher.is_symlink():
    previous = (
        launcher.read_text() if launcher.is_file() and not launcher.is_symlink() else ""
    )
    legacy = f'#!/bin/sh\nexec {shlex.quote(str(entry))} "$@"\n'
    if not previous.startswith(f"#!/bin/sh\n{marker}\n") and previous != legacy:
        raise SystemExit(
            f"{launcher} already belongs to another installation. Choose CARLA_BIN_DIR or move it first."
        )

folder.mkdir(parents=True, exist_ok=True)
launcher.write_text(content, encoding="utf-8")
launcher.chmod(0o755)
print(f"Installed {launcher}. Run: carla")
paths = [Path(p).expanduser().resolve() for p in os.get_exec_path()]
if folder.resolve() not in paths:
    export = f'export PATH={shlex.quote(str(folder))}:"$PATH"'
    print(f"Add this line to your shell profile, then open a new terminal:\n  {export}")
