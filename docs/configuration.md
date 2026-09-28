# Configuration

## Models

Carla opens its model setup dialog when no configured base GGUF exists.
Use arrows and Enter to select, and Escape to go back one step or skip setup.
File paths and URLs are entered in the same input controls as other Carla dialogs.
`carla --setup-model` opens setup explicitly; inside Carla, use `/model` →
`+ Add model`. An explicit `--models FILE` bypasses automatic detection.
Both the installed launcher and direct TUI binary use this same setup flow.

The preset menu offers the Qwen3 8B, 14B and 30B-A3B Base Q4_K_M files used in local
experiments, pinned to those revisions. Their GGUF sizes are approximately 4.68,
8.38 and 17.28 GiB; these are disk sizes, not RAM requirements. This is a compatibility
list, not a character-quality benchmark. Custom sources accept `owner/repo`, a
Hugging Face repository URL, or a direct GGUF blob/resolve URL. Repository choices
list GGUF files; a split selection downloads every shard and registers the first.
The selected revision resolves to a commit before downloading.

The setup displays file size, license metadata and the source repository, then asks before
downloading. It uses [Hugging Face Hub's download/cache support](https://huggingface.co/docs/huggingface_hub/guides/download)
under `$CARLA_DATA_DIR/models`. Retry the same selection after interruption to reuse
cached progress. For gated/private repositories, authenticate with Hugging Face or
set `HF_TOKEN`; credentials are not saved in Carla's model configuration. A local
GGUF is used in place. GGUF headers and shard presence are checked; custom model
training type must be identified by the user. Base models go into `models.json`;
an instruct model can be configured as the separate Grow selector. An existing
policy model is not silently replaced.

Metadata checks and downloads run in the background. Download progress appears
inside Carla; Escape cancels the transfer and retains cached partial files.
Errors appear in the same dialog, with a way back to edit the source.
Setup registers a model only after successful acquisition. It leaves existing
entries intact and selects a distinct configured port, though another external
process can still occupy that port. Initial context is 8192 and GPU layers 99;
adjust these in settings/configuration for your hardware. No RAM-fit or throughput
claim is inferred from the download size. Skip remains available for offline
browsing. The model manager does not install llama.cpp.

`--models FILE` accepts a nonempty JSON array of base models. Paths expand `~`;
relative paths resolve against the configuration file, not the working directory.
Each entry needs `alias`, `kind: "base"`, `path` and `port`. Optional fields are
`name`, `context`, `gpu_layers`, `url` and provenance metadata such as `source`.

- Aliases must be unique. Endpoints must be loopback HTTP and match the configured
  port; use distinct ports for distinct models.
- `context` defaults to 8192. Zero requests llama.cpp's model default. Large native
  contexts can exhaust memory; choose a capacity your machine can hold. Output
  budgets are separate and never silently reduced to admit more requests.
- `gpu_layers` defaults to 99; use 0 for CPU or a supported layer count for your
  hardware. GPU support depends on how llama.cpp was built.
- Carla launches with `--parallel 4 --kv-unified --cont-batching`, disables idle
  slot caches and context shifting, and checks prompt length before sampling.
  Use a llama.cpp build whose `llama-server --help` supports these flags plus
  `--no-cache-idle-slots` and `--cache-ram`. Older builds are not silently adapted.
- A running server is reused only if its reported alias matches. Externally
  managed servers receive conservative single-request admission; Carla will not
  terminate them to switch models. Stop them yourself before multi-model runs.

For persistent defaults, place the array at `$CARLA_DATA_DIR/models.json`.
Saved workspace model entries override matching registry entries; an explicit
`--models FILE` replaces the workspace catalog. `/model` selects from that catalog.

Repeated Loom loops need a separate instruct selection model. `--policy-model FILE` accepts one object
with the same fields and `kind: "instruct"`. Setup saves the persistent default at `$CARLA_DATA_DIR/policy-model.json`. Selection unloads
the generator before loading the selector. Selection instructions never enter
raw generation context. Without a configured policy model, multi-loop Loom cannot run.

## Storage and launch

`CARLA_DATA_DIR` defaults to `~/.local/share/character-lab`. Workspaces, the shared
library, registry and keybindings live there. Use `--workspace NAME` to create or
open a named workspace, `--project PATH` for an explicit directory, or no flag to
resume the last workspace. The workspace picker is also available inside Carla.
One process owns each workspace via an exclusive lock.

A workspace contains `project.json`, optional in-flight `stream.jsonl`, exported
snapshots and model-server logs. Copy the whole directory for backup. A journal
record contains only the new text/provider events. Checkpoints record its sequence
before removing the journal; recovery skips already-checkpointed records.
A truncated final append is ignored; complete malformed records fail visibly.

`make install` creates a launcher in `~/.local/bin` pointing to this checkout.
Set `CARLA_BIN_DIR` when installing to choose another launcher directory. Re-run
`make install` after moving the checkout; the launcher does not copy the app.

The Python entry point locates the Go binary at this source checkout's `bin/carla`.
For a separately installed Python package, set `CARLA_BINARY` to the built Go
binary. Direct binary launches use the checkout's `.venv/bin/python`; override
with `CARLA_PYTHON` when needed. The documented installation is from source.

## Document library

The library is shared across workspaces, but loading a workspace does not select
all its documents. In the TUI, use `/import` or **+ Add document** in Library.
Enter a local UTF-8 `.txt` or `.md` file path, an optional title (defaults to the
filename), and optional author/source URL. Tab or Enter moves between fields; Enter on the last field imports.
Ctrl+Enter imports from any field; Escape cancels. The document appears immediately, without selecting it as a seed.

Alternatively, import from the command line:

```sh
uv run python -m character_lab.library ./seed.txt \
  --key my-seed --title "My seed" --author "Author name"
```

Optional `--source-url` records provenance. Import retains the exact decoded text
and source SHA-256. Existing keys cannot be overwritten through this command.
Restart Carla to load a changed library. Existing branches retain their copied
source text even if the library changes.

For numbered books or curated excerpts, place JSON documents under
`$CARLA_DATA_DIR/library/`. Files are read in filename order. Example:

```json
{
  "key": "my-book",
  "title": "My book",
  "author": "Author name",
  "url": "https://example.org/original",
  "normalization": "Describe any extraction or edits here.",
  "passages": [
    {"id": "1.1", "book": 1, "number": 1, "text": "First passage."},
    {"id": "1.2", "book": 1, "number": 2, "text": "Second passage."}
  ]
}
```

Keys and passage IDs must be unique within their scope. `book`/`number` enable
nested numbered selection; arbitrary passages can instead supply a `label`.
The bundled library contains Gunkel’s Paths table, 487 numbered Meditations
passages and the seven main Tractatus propositions. These load offline; local
entries with matching keys override the bundled entries. See
[source attribution](../src/character_lab/seeds/README.md) for scope and rights.
The texts are not covered by Carla’s MIT software license.

## Optional monitoring

The Monitoring policy under `/policy` configures Jev via OpenRouter System One. Enable it only when you
intend to send the full conversation history, including character output and
human messages, to that external service. Its requests may incur charges.
Monitoring defaults to Off. Choose Jev to open a masked OpenRouter API-key form;
other monitoring controls appear only after a key is configured. Cancelling this
step leaves monitoring Off. An existing `OPENROUTER_API_KEY` in the launching
environment also satisfies setup.

Keys entered in Carla are saved in `credentials.json` under its application data
home, outside all workspaces, with owner-only permissions (`0600`). This is a
local file, not encrypted storage. A saved key takes precedence over the
environment variable; the API key row lets you replace it. Turning monitoring Off
retains the key for later use. Keys are not included in state events, generation
configs, traces or dataset exports. Saving a key does not make a paid request or
verify provider authentication; provider errors remain visible when used.

Checks apply to character replies and document continuations. In `/policy` →
Monitoring → When to check, toggle after-reply and during-reply checks separately.
Both default to on, with a 512-output-token interval when monitoring is enabled.
Turning during-reply checks off preserves the interval. Legacy interval 0 still
disables mid-reply checks. Only one check per
conversation is in flight. Partial checks do not block token streaming; the
conversation awaits its pending/final result before advancing so an explicit Stop
can take effect. Sibling conversations on the same model can keep advancing.
Provider errors fail open and remain visible. Scores are provider classifications,
not a locally calibrated guarantee. A Stop affects only the flagged conversation.

## Policy and evaluation configuration

Use `/policy` for monitoring, selection, and saved evaluations. `/config` now
contains only generation settings. Each saved evaluation chooses a local LLM or
Jev judge, criteria, and (for Jev) a pass-probability threshold. Local judging also
exposes its complete system prompt. Definitions are workspace-local and revisioned;
existing results keep the definition used at execution time. See
[commands](commands.md#policies-and-evaluated-datasets) for targeting and exports.
