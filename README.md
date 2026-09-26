# Carla

Carla (character lab) is a TUI for developing AI characters from base models inspired by [Computer-10](https://x.com/parafactual/status/2102611793369821277?s=20)

Select from seed documents to generate base-model continuations, branch, edit, and curate an anthology of generations and explore how it behaves in simulated conversations. Prompts, model settings, source provenance and alternative branches remain inspectable.

```text
Library → Branches → Anthology → Simulator
             │                      │
         fork / Loom           fork / Loom
             └── inspect, edit, compare ──┘
```

## Quick start

Requirements: Python 3.11+, [uv](https://docs.astral.sh/uv/), Go 1.26+,
and a UTF-8 terminal of at least 60 × 18 cells. Development is on macOS;
Linux is included in CI but has not had equivalent interactive validation.
Native Windows is unsupported (Unix workspace locking and process replacement).

Clone the repository, enter its directory, and launch Carla:

```sh
git clone https://github.com/k3nnethfrancis/carla.git
cd carla
make install
carla
```

`make install` prepares the Python environment, builds the terminal interface,
and installs a launcher in `~/.local/bin`. If that directory is not on your PATH,
the installer prints the line to add to your shell profile.

After that, run `carla` from any directory to resume your last workspace.
Use `carla --workspace first-experiment` to create or open a named workspace.

On first launch, Carla opens a setup dialog if no configured GGUF is available.
Use ↑/↓ and Enter to choose, or Escape to go back:

- Download a tested Qwen3 **8B**, **14B**, or **30B-A3B Base** GGUF (Q4_K_M).
- Paste a Hugging Face repository or GGUF file URL and choose a file to download.
- Point to a local GGUF without copying it.
- Skip setup to browse the original example document offline.

Downloads show their size, license and pinned revision before confirmation.
Use `/model` → `+ Add model`, or run `carla --setup-model`, to add another model. Generation also requires
`llama-server` on PATH; see [generation setup](#set-up-generation).

Once inside Carla, select a passage with Space, open it with Enter, and explore the
panes with Tab / Shift+Tab. Type `/help` for commands or `/keys` for editable
bindings. `/` focuses the command bar, including from the document editor.

## Commands at a glance

Type `/` anywhere to open the command bar. Start typing, use ↑/↓ to select a
suggestion, and press Enter. Commands relevant to the current page appear first.

| Command | What it does |
| --- | --- |
| `/help` · `/keys` | Browse all commands or customize keyboard bindings. |
| `/workspace` · `/model` | Choose a workspace or local model. |
| `/configure` | Open settings for the current page. |
| `/continue --tokens 512` | Continue a document from the cursor, up to 512 output tokens. |
| `/loom 3 --tokens 512` | In Branches, generate three alternative continuations. |
| `/loom 3 --turns 4 --tokens 512` | In Simulator, generate three conversations, each with four new character replies. |
| `/branch` | Fork the selected document or conversation without generating. |
| `/keep` · `/anthology` | Keep selected branches or browse the curated anthology. |
| `/edit` · `/save` · `/cancel` | Edit, save a new version, or discard the draft. |
| `/inspect` · `/notes` | Inspect exact inputs and provenance, or open document notes. |
| `/simulator` · `/run` | Open Simulator or start fresh conversations with its settings. |
| `/grid` · `/active` | View Loom outputs together or jump to active generation. |
| `/grow` · `/grow-policy` | Explore branches with a local selector, or edit its criteria. |
| `/loom-policy` | Configure optional conversation monitoring; off by default. |
| `/snapshot` | Export anthology documents and their provenance. |
| `/stop` · `/restart` · `/exit` | Stop generation, restart Carla, or exit. |

`/help` has the full list. Generation needs a configured model; `--tokens` sets
an output ceiling, and `--turns` counts character replies rather than both speakers.

## Set up generation

To generate, install [llama.cpp](https://github.com/ggml-org/llama.cpp) with
`llama-server` on PATH and supply a **base-model GGUF** you are licensed to use.
The server must support unified KV cache and continuous batching; see
[model setup](docs/configuration.md#models). The first-run setup writes model
configuration for you. To add a model later:

```sh
carla --setup-model
```

GGUF is a file format; quantization is optional. Model weights are never bundled
or downloaded automatically. A missing model does not prevent source browsing.

## Work through an experiment

- **Library:** select passages from shared documents; only selected text enters
  the workspace. [Import your own text](docs/configuration.md#document-library).
- **Branches:** `/continue --tokens 512` samples one continuation from the cursor;
  `/loom 3 --tokens 512` samples three alternatives. `/branch` forks the current
  version without generating. Edits preserve ancestry.
- **Anthology:** `/keep` retains a document for curation. Keeping is a human
  selection, not an automatic quality verdict or training step.
- **Simulator:** `/configure` chooses documents, speakers, openings and sampling.
  `/loom 3 --turns 4 --tokens 512` produces three conversations with four new
  character replies each. A visitor replies between character turns. Selecting
  an existing conversation resumes its frozen document context and history.
  `/run` starts fresh. Multi-output runs stream into a selectable grid.

`/grow` generates candidate branches and asks a separate local instruct model to
select which to explore next. `/grow-policy` exposes that selection specification.
It never silently accepts documents into the anthology. Configure its model with
`--policy-model`; see [configuration](docs/configuration.md).

Optional `/loom-policy` monitoring sends conversation history to Jev through
OpenRouter. It is **off by default**. Warn and Stop actions are explicit per
condition; built-in actions default to Warn. This is separate from Grow selection.
See [data and monitoring](docs/configuration.md#optional-monitoring) before enabling it.

## What to expect

- Inference uses local llama.cpp raw completions. Base-model continuations have
  no hidden assistant prompt, RAG memories or reflection step. Simulator templates
  are explicit and inspectable; the Grow selector uses a separate chat endpoint.
- Up to four requests share one resident model, subject to memory and full
  prompt/output context reservations. This is a conservative heuristic, not an
  optimal throughput scheduler. Large budgets can serialize requests. Same-model
  conversations advance independently; different model aliases require barriers
  for unloading/loading weights.
- `--tokens` is an output ceiling, not a required length. `Max` reserves the
  remaining context, often leaving room for only one request. EOS or an explicit
  speaker boundary can end a reply earlier. Token-limit observations do not end
  later conversation turns.
- Quality is an open research question. Base models can imitate source authors,
  drift, loop, or produce disturbing content. An anthology is context for the
  simulator, not evidence that a character has been learned in weights.
- Workspaces retain raw prompts and outputs. They can become large; streaming
  uses an incremental journal, but full checkpoints still occur at turn and
  operation boundaries. Recovery covers process crashes, not guaranteed survival
  of power loss. Back up valuable workspaces.
- The app has no training jobs, calibrated judge dashboard, multi-user service,
  remote deployment, or automatic research conclusions.

## Develop and contribute

```sh
make test
make lint
make build
```

Tests use synthetic documents and fake inference; they need no weights, GPU or
API keys. See [architecture](docs/architecture.md) for code boundaries and
[research references](docs/resources.md) for method context.

Code is [MIT licensed](LICENSE). Imported texts, model weights and generated
artifacts retain their own applicable terms. No third-party seed corpus is bundled.
For optional hosted monitoring and stored trace details, see
[configuration](docs/configuration.md#optional-monitoring).
