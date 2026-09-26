"""Workspace discovery and exclusive ownership, independent of any interface."""

import fcntl
import json
import os
from pathlib import Path

HOME = Path(
    os.environ.get("CARLA_DATA_DIR", Path.home() / ".local/share/character-lab")
)


class Workspaces:
    def __init__(self, home=HOME):
        self.home = Path(home)
        self.home.mkdir(parents=True, exist_ok=True)
        self.registry = self.home / "workspaces.json"

    def read(self):
        if not self.registry.exists():
            return {"paths": [], "last": None}
        return json.loads(self.registry.read_text())

    def remember(self, folder):
        data = self.read()
        path = str(folder.resolve())
        data["paths"] = list(dict.fromkeys([*data["paths"], path]))
        data["last"] = path
        tmp = self.registry.with_suffix(".tmp")
        tmp.write_text(json.dumps(data, indent=2) + "\n")
        tmp.replace(self.registry)

    def list(self, current):
        paths = {current.resolve()}
        for root in {self.home, current.parent}:
            paths.update(p.parent.resolve() for p in root.glob("*/project.json"))
        paths.update(
            Path(p)
            for p in self.read()["paths"]
            if (Path(p) / "project.json").is_file()
        )
        return [dict(name=p.name, path=str(p)) for p in sorted(paths)]

    def named(self, name):
        if not name.strip() or name in {".", ".."} or Path(name).name != name:
            raise ValueError("Use a workspace name, not a path")
        return self.home / name


def acquire(folder):
    folder.mkdir(parents=True, exist_ok=True)
    lock = (folder / ".lock").open("a")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        lock.close()
        raise ValueError("Workspace is already open in another process") from None
    return lock
