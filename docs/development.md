# Developing Carla

GitHub Issues is the canonical bug and feature backlog; access follows repository
permissions. Repository visibility and release publication are explicit maintainer
decisions. See the [release-readiness checklist](#public-release-readiness).

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

Every promotion to `main` is a release. Ordinary feature/fix merges into `dev`
do not tag or publish anything.

1. Before opening the `dev` → `main` PR, bump `project.version` in
   `pyproject.toml` through a PR into `dev`, then run `uv lock` to update the
   lockfile. The initial release uses `0.1.0`; subsequent compatible fixes use
   `0.1.1`, `0.1.2`, etc. Document the user-visible changes in the PRs.
2. The promotion PR's **Release / validate** check rejects a reused or older
   version. Review the changes and wait for all checks before merging.
3. A push to `main` starts **Release**, which tests/builds that exact commit on
   macOS and Linux, including the source installer and launcher. Only after those
   checks pass does it create `v<version>` and publish the GitHub release with
   generated notes. Review and expand those notes for significant releases.

Tags never move. Re-running **Release** on the same `main` commit safely leaves
an existing published release alone, or completes publication if only its tag
exists. If checks fail, fix them through `dev` and promote again; if publication
alone fails, rerun the workflow on `main`. The publisher refuses to release a
stale commit if `main` moved during checks. API failures stop the release rather
than being interpreted as missing tags. Manually created untagged drafts must be
resolved before the workflow can claim their version.

The private repository currently cannot enforce branch protection; passing the
version check is a required maintainer convention. A direct push with a reused
version will fail publication, not retag an existing release.

Initial releases are source releases: clone/checkout the tag, run `make install`,
then `carla`. A Python wheel alone does not contain the Go frontend; no standalone
binary installer or public package-registry release is promised here. There are no
model weights or user workspaces in release assets. GitHub Release notes are the
changelog; avoid maintaining a duplicate one in the repository.

Merging into `main` authorizes release publication through this workflow. It never
changes repository visibility; a private repository’s releases remain private.

## Public-release readiness

Before changing visibility or publishing a release:

- Verify redistribution terms before adding starter texts; retain edition and
  source provenance. Gunkel’s Paths table is a source reference only, not bundled.
- Preserve third-party notices, including the vendored TUI skill's MIT license.
- Confirm no credentials, private workspaces, model weights or research artifacts
  are tracked. Use synthetic fixtures in tests and examples.
- Run the documented install and changed terminal journeys, plus all CI checks.
- Keep implemented capabilities, known limits and future training work distinct
  in the README. Publish the intended reviewed `main` revision.

Documentation completion does not itself change repository visibility.

## Command documentation

`tui/command_registry.go` owns command names, compatibility aliases, default action
labels/bindings, descriptions and argument/help text. Help and command completion
reuse it. Contextual availability and execution remain with their existing handlers;
this registry does not duplicate the backend command parser.

After changing metadata, regenerate the reference:

```sh
cd tui
CARLA_UPDATE_COMMAND_DOCS=1 go test -run TestCommandReferenceMatchesRegistry
```

Normal tests verify that `docs/command-reference.md` matches the registry and every
registered action has a canonical description. Keep README's first-experiment
walkthrough short and link to the reference for the full command scope.
