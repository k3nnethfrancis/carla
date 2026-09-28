"""Validate release versions and publish only the commit checked by release.yml."""

import json
import os
import re
import subprocess
import sys
import tomllib
from pathlib import Path


def release_tag(version, sha, tags):
    """A version advances monotonically; retries may reuse only the same commit."""
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("Use a numeric major.minor.patch app version")
    tag = "v" + version
    if tag in tags:
        if tags[tag] != sha:
            raise ValueError(
                f"{tag} already identifies another commit; bump the version"
            )
        return tag
    previous = [
        tuple(map(int, name[1:].split(".")))
        for name in tags
        if re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", name)
    ]
    if previous and tuple(map(int, version.split("."))) <= max(previous):
        raise ValueError(
            "The app version must be newer than every existing release tag"
        )
    return tag


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True)


def api(path):
    return json.loads(gh("api", "repos/" + os.environ["GH_REPO"] + path))


def main():
    with Path("pyproject.toml").open("rb") as stream:
        version = tomllib.load(stream)["project"]["version"]
    sha = os.environ["GITHUB_SHA"]
    pages = json.loads(
        gh(
            "api",
            "--paginate",
            "--slurp",
            f"repos/{os.environ['GH_REPO']}/tags?per_page=100",
        )
    )
    tags = {tag["name"]: tag["commit"]["sha"] for page in pages for tag in page}
    tag = release_tag(version, sha, tags)
    releases = json.loads(
        gh(
            "api",
            "--paginate",
            "--slurp",
            f"repos/{os.environ['GH_REPO']}/releases?per_page=100",
        )
    )
    existing = next(
        (r for page in releases for r in page if r["tag_name"] == tag), None
    )
    if existing and tag not in tags:
        raise ValueError(f"{tag} has an untagged draft; resolve it before releasing")
    print(f"Validated {tag} at {sha}")
    if "--publish" not in sys.argv:
        return
    if os.environ["GITHUB_REF"] != "refs/heads/main":
        raise ValueError("Releases can only be published from main")
    if api("/commits/main")["sha"] != sha:
        raise ValueError("main moved during checks; rerun on current main")
    if existing:
        if existing["draft"]:
            gh("release", "edit", tag, "--draft=false", "--latest")
        else:
            print(f"{tag} is already published; nothing to change")
        return
    gh(
        "release",
        "create",
        tag,
        "--target",
        sha,
        "--title",
        "Carla " + tag,
        "--generate-notes",
        "--latest",
    )


if __name__ == "__main__":
    main()
