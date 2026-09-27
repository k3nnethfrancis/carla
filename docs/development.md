# Developing Carla

Carla is currently shared privately with collaborators. GitHub Issues is the
canonical bug and feature backlog; access follows repository permissions.

## Work on a change

1. Open or find a focused issue describing the problem, desired behavior and
   acceptance criteria. Include reproduction steps for a bug. Small documentation
   corrections can go straight to a PR.
2. Create a short-lived feature/fix branch from current `origin/dev`. Keep unrelated
   changes in separate PRs; explicitly target `dev` when opening them.
3. Read [architecture](architecture.md) and [AGENTS.md](../AGENTS.md). For terminal
   work, use the repository's [TUI design skill](../skills/tui-design/SKILL.md).
4. Run the relevant checks while iterating, then `make test lint build`. UI changes
   also need the affected keyboard journey exercised in a real terminal. Use
   disposable data, not a collaborator's active workspace.
5. Open a PR into `dev` linking the issue, explaining the resulting behavior and verification.
   Wait for passing GitHub checks and maintainer review before merging. Squash merge
   keeps one logical change per feature PR; delete the feature branch afterward.

```sh
git fetch origin
git switch -c feat/short-description origin/dev
# Commit and push the focused change, then:
gh pr create --base dev
```

`dev` is the integration branch. `main` is the reviewed installation/release branch
and remains GitHub's default, so new clones start with the reviewed version. Checks
run on PRs and pushes to both branches. Promote a tested set of work with a separate
`dev` → `main` PR, using a merge commit to retain integration ancestry. Keep both
long-lived branches; branch deletion applies only to feature/fix branches. Releases
still run only from `main`, after promotion.

Routine changes go through PRs rather than direct pushes to `main` or `dev`. This is the
collaborator workflow, not a claim that branch protection is enabled: the current
private repository's plan does not support the requested ruleset API. Revisit
mechanical enforcement if the plan or repository visibility changes.

Use `bug`, `enhancement` and `documentation` labels for work type. Add `ready` when
an issue is actionable and `blocked` only with a concrete dependency described in
its body. The current milestone is
[Data generation stabilization](https://github.com/k3nnethfrancis/carla/milestone/1).

## Current scope

Make the existing data-generation stage dependable: Library/source selection,
branching and continuation, editing/notes, anthology curation and simulated
conversations. Tighten the UX, resolve reproducible bugs, and verify persistence,
long-running work and installation before expanding the pipeline.

Review the discovery-driven implementation before deciding what needs refactoring.
Accepted findings should become focused issues and small PRs with behavior evidence;
a broad rewrite is not the default. The milestone links UX, architecture and
reliability reviews so collaborators can make those decisions together.

Later training and research phases are on hold pending collaborator discussion.
A public launch is a separate decision, not a scheduled consequence of this work.

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
