# Developing Carla

Carla is currently shared privately with collaborators. GitHub Issues is the
canonical bug and feature backlog; access follows repository permissions.

## Work on a change

1. Open or find a focused issue describing the problem, desired behavior and
   acceptance criteria. Include reproduction steps for a bug. Small documentation
   corrections can go straight to a PR.
2. Create a short-lived branch from `main`. Keep unrelated changes in separate PRs.
3. Read [architecture](architecture.md) and [AGENTS.md](../AGENTS.md). For terminal
   work, use the repository's [TUI design skill](../skills/tui-design/SKILL.md).
4. Run the relevant checks while iterating, then `make test lint build`. UI changes
   also need the affected keyboard journey exercised in a real terminal. Use
   disposable data, not a collaborator's active workspace.
5. Open a PR linking the issue, explaining the resulting behavior and verification.
   Wait for passing GitHub checks and maintainer review before merging. Squash merge
   keeps one logical change per PR; delete the branch afterward.

Routine changes go through PRs rather than direct pushes to `main`. This is the
collaborator workflow, not a claim that branch protection is enabled: the current
private repository's plan does not support the requested ruleset API. Revisit
mechanical enforcement if the plan or repository visibility changes.

Use `bug`, `enhancement` and `documentation` labels for work type. Add `ready` when
an issue is actionable and `blocked` only with a concrete dependency described in
its body. Milestones can group agreed outcomes; no roadmap is imposed by this setup.

## Repository structure

| Location | Owns |
| --- | --- |
| `src/character_lab/` | Python domain state, persistence, local inference and jobs |
| `tui/` | Go terminal UI, commands, layout and interaction tests |
| `tests/` | Python regression tests using synthetic data/fake inference |
| `scripts/`, `.github/` | Build/install helpers, issue forms and CI/release workflows |
| `docs/` | Current user and developer contracts |
| `skills/tui-design/` | Carla-specific, provider-neutral TUI guidance |

Research notes, working plans, generated traces, user workspaces, model weights
and credentials stay outside the tracked repository. Publish research separately
when ready. Keep fixtures small and synthetic. Skills do not execute automatically
or supply runtime character prompts; point your coding environment to `SKILL.md`.

## Checks

```sh
uv sync --locked
make test lint build
./bin/carla --version
uv run carla --version
```

Tests need no model weights, GPU or API keys. CI repeats checks on macOS and Linux.
`make build` stamps the frontend with the version from `pyproject.toml` and the Git
revision (plus `-dirty` for a changed checkout). Plain `go build` reports `dev`;
use `make build` for a versioned binary. The installed launcher runs that binary.
`--version` exits without opening a terminal UI, workspace or inference engine.

## App releases

`pyproject.toml` is the app version source of truth. While Carla is experimental,
use `0.1.x` for compatible fixes and a new minor version for a coherent new
capability or a breaking change. Describe compatibility changes explicitly even
before 1.0. A release tag is `v` followed by the manifest version; tags are immutable.

For each release:

1. Submit the version change in a PR (the first release can use the existing
   `0.1.0`). Run `uv lock` to update the package version in the lockfile. Document
   user-visible changes and any relevant limitations in the PR.
2. Once merged, run **Draft release** from GitHub Actions on `main`, supplying the
   exact manifest version. It checks the version and tests/builds the exact commit
   on both supported CI platforms before creating a draft GitHub Release.
3. Review the generated release notes and install the draft's exact source commit
   in a separate checkout. Publish the draft only after that review.

Initial releases are source releases: clone/checkout the tag, run `make install`,
then `carla`. A Python wheel alone does not contain the Go frontend; no standalone
binary installer or public package-registry release is promised here. There are no
model weights or user workspaces in release assets. GitHub Release notes are the
changelog; avoid maintaining a duplicate one in the repository.

Creating the workflow does not publish a release or change repository visibility.
