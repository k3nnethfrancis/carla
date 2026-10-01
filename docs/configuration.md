# Configuration

## Models

Carla opens its model setup dialog when no configured base GGUF exists.
Use arrows and Enter to select, and Escape to go back one step or skip setup.
File paths and URLs are entered in the same input controls as other Carla dialogs.
`carla --setup-model` opens setup explicitly; inside Carla, use `/config` → models →
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
an instruct model can be configured as the separate selection-policy model. An existing
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
`--models FILE` replaces the workspace catalog. The model picker in `/config` selects from that catalog.

Optional policy-guided split loops need a separate instruct selection model. `--policy-model FILE` accepts one object
with the same fields and `kind: "instruct"`. Setup saves the persistent default at `$CARLA_DATA_DIR/policy-model.json`. Selection unloads
the generator before loading the selector. Selection instructions never enter
raw generation context. Selection defaults to Off. Ordinary loops continue each output without a judge;
Selection On requires a configured policy model for split loops.

## Prompts and templates

Carla uses llama.cpp raw completions for document and conversation generation.
It does not apply a hidden assistant system prompt or the model's chat template.
Selection and local evaluation are separate instruct-model judge calls.

| Input | Editor | How it is used |
| --- | --- | --- |
| Document continuation | Branches → `/edit`, save, position cursor | Exact document prefix up to the cursor; no separate wrapper/template setting. |
| Character template | Simulator → `/config` → Character prompt | Formatted with frozen anthology text and conversation history. |
| Visitor template | Simulator → `/config` → Visitor prompt | Formatted with Visitor brief and conversation history by default. |
| Visitor brief | Simulator → `/config` → Visitor brief | Text substituted into `{visitor_brief}`. |
| Fixed opener | Simulator → `/config` → Opening → Fixed → Message | First Visitor message for fresh conversations; `--msg` overrides it for one run. |
| Generated opener | Simulator → `/config` → Opening → Generated → Generation prompt | Raw completion using its selected model/sampling, once per fresh conversation. |
| Selection | `/policy` → Selection | Criteria and system prompt for candidate classification; never injected into generator text. |
| Monitoring | `/policy` → Monitoring → a policy → Behaviors | Named behavior specs sent to local DiffusionGemma or Jev with context. The provider envelope is managed by Carla. |
| Evaluation | Evaluate → Policies | Criteria and local judge system prompt, or DiffusionGemma/Jev behavior spec/threshold. |

### Document continuations

The input is exactly the selected document's text before the cursor. Editing or
forking that text changes the next input; moving the cursor changes where the
continuation begins. There is no separate continuation-template editor today.
This preserves raw base-model exploration. `/inspect` exposes the saved prefix,
sampling settings, model and generation trace. Source text and human/AI edits
remain attributable through ancestry.

### Conversation templates

Current defaults are:

Character:

```text
{anthology}

Full conversation with Model C:

{history}

**Model C:**
```

Visitor:

```text
{visitor_brief}

Full conversation with Model C:

{history}

**User:**
```

Both accept `{anthology}`, `{history}` and `{visitor_brief}`. `{history}` is
required in both; `{anthology}` is required in the Character template. Unknown
fields or invalid formatting are rejected. Use doubled braces `{{` and `}}` for
literal braces. These are Python string-format fields, not executable templates.

History is rendered as `**User:**` and `**Model C:**` turns. Speaker-boundary stop
strings use those labels too. They are not separately configurable: preserve
this convention when editing templates. A template can change surrounding prose
and where the available fields appear, but it cannot redefine the history renderer
or add arbitrary new variables through configuration.

The default Visitor brief is `A curious visitor talks with Model C.` The default
fixed opener is `What would you like to talk about?`. Generated openings have an
editable prompt and sampling; **Preview 3 openings** tests just that stage. The
opening generator does not receive anthology text automatically. A blank opening
model selection follows the configured Visitor model.

Fresh runs freeze selected anthology versions and effective configuration. A
continuation of an existing conversation keeps its frozen document context and
history; later settings affect new generation without rewriting prior prompts.
Use `/inspect` to examine the fully rendered input rather than inferring it from
the template alone. No RAG recall, reflection pass or hidden character memory is
added. Configuration changes do not alter saved traces.

### Judge prompts

The selection prompt must preserve its JSON contract: review each candidate once,
include valid evidence, and return an eligible candidate ID or null. The local
evaluation prompt must return `passed` (boolean), `reason` and `evidence`. Full
contracts/defaults are visible in their editors and in saved judge requests.
Monitoring allows behavior specs, thresholds and actions; its transport envelope
is not a free-form prompt editor. See [policies and evaluations](evaluations.md).

### Sampling and overrides

Document `/config` exposes model and shared sampling/context settings. Simulator
has separate Character, Visitor and generated-opening sampling. Explicit Loom
flags override one run; they do not rewrite saved configuration. Bare Loom always
uses one alternative and one loop, plus one Character reply in Simulator.

`--tokens N` caps new output tokens; it is not a minimum length. `Max` uses the
remaining available context. EOS and speaker boundaries may end output sooner.
Simulator `--tokens` overrides both Character and Visitor ceilings, while the
opening generator keeps its own settings. `--turns` counts Character replies per
alternative per loop. `--loops` repeats continuation; Selection On instead enables policy-guided split loops. See the
[complete syntax](commands.md#generation-arguments).

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
all its documents. In the TUI, use `/add` or **+ Add document** in Library.
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
The bundled library contains 487 numbered Meditations
passages and the seven main Tractatus propositions. These load offline; local
entries with matching keys override the bundled entries. See
[source attribution](../src/character_lab/seeds/README.md) for scope and rights.
The texts are not covered by Carla’s MIT software license.

## Optional monitoring

Monitoring is Off by default. For an entirely local classifier, select
**DiffusionGemma (local)** and follow [local judge setup](local-judge.md).
After the one-time installation, Carla starts the cached local judge automatically
on an available loopback port and stops it on exit. No server address or API key
is needed. Its model setting is separate from the hosted Jev settings below.

The Jev option uses OpenRouter System One. Choose it only when you
intend to send the full conversation history, including character output and
human messages, to that external service. Its requests may incur charges.
Monitoring defaults to Off. Choose Jev to open a masked OpenRouter API-key form;
its other monitoring controls appear only after a key is configured. Cancelling this
step leaves monitoring Off. An existing `OPENROUTER_API_KEY` in the launching
environment also satisfies setup.

Keys entered in Carla are saved in `credentials.json` under its application data
home, outside all workspaces, with owner-only permissions (`0600`). This is a
local file, not encrypted storage. A saved key takes precedence over the
environment variable; the API key row lets you replace it. Turning monitoring Off
retains the key for later use. Keys are not included in state events, generation
configs, traces or dataset exports. Saving a key does not make a paid request or
verify provider authentication; provider errors remain visible when used.

**Call mode** defaults to **Separate**: one request per enabled behavior,
using the same captured text. **Bundled** asks all enabled behaviors in one
request. Choosing Bundled opens a confirmation warning: it can reduce calls,
cost and latency, but asking behaviors together can change scores or miss
behaviors. Switching back to Separate takes effect directly. Each score remains
an independent yes/no probability in either mode; the scores do not sum to one.
Missing settings in older workspaces use Separate; explicit Bundled choices are
retained. Historical traces are unchanged.

Separate calls run sequentially within a check. Traces retain each request,
response, timing and error. If one behavior fails, the check is marked partial;
successful behavior scores and their configured actions still apply. A failed
behavior is never treated as a negative score or a reason to stop. This setting
covers live monitoring, not named evaluation judge execution.

Checks apply to character replies and document continuations. In `/policy` →
Monitoring → a policy → Heartbeat, toggle after-reply and during-reply checks separately.
Both default to on, with a 512-output-token interval when monitoring is enabled.
Turning during-reply checks off preserves the interval. Legacy interval 0 still
disables mid-reply checks. Only one check per
conversation is in flight. Partial checks do not block token streaming; the
conversation awaits its pending/final result before advancing so an explicit Stop
can take effect. Sibling conversations on the same model can keep advancing.
Provider errors fail open and remain visible. Scores are provider classifications,
not a locally calibrated guarantee. A Stop affects only the flagged conversation.

## Policy and evaluation configuration

`/policy` always opens Monitoring, Selection and Evals, from every tab.
Choose a named policy within a category to edit it. Monitoring and Selection
use On/Off; editing an Off policy does not enable it. Evaluate → Policies
opens the same Evals policy list; there is no separate evaluation configuration.
Policies own behaviors separately from their judge settings. The judge owns the
model, prompt and Call mode. Behaviors own specs, enabled states and passing
rules. Every judge in an evaluation policy assesses the same enabled behaviors.
Models can be the configured local LLM, local DiffusionGemma or hosted Jev.
Classifiers use probability thresholds; local LLM judges expose the full system
prompt and return a boolean judgment with evidence for each behavior. DiffusionGemma uses the
managed local OpenJev worker after [one-time setup](local-judge.md).

Data collections and policies are independent. Choose an active policy for bare
`/eval`, or use `/eval "policy name"`. `/config` handles the current view's
settings. `/behaviors` opens the workspace spec library. Importing into a policy copies the
spec with its library revision; enabled states, detection rules and actions remain
local to the policy. Saved runs
retain the configurations used at execution. Adding data never invokes a model.
See [evaluations](evaluations.md) for the Data / Policies / Runs workflow.

Named Monitoring and Selection policy rows show only **On/Off**. Space or
Left/Right toggles the focused policy; Enter opens its configuration. Turning
one On switches the previous policy in that category Off and uses the newly
enabled policy for future runs. All policies can be Off. New policies start
Off; their judge and behavior settings remain saved when disabled. Enabling
monitoring uses its remembered judge, with model/key setup when needed.
Cancelling setup leaves the previously enabled policy unchanged.

Choice rows in policy configuration support Space to cycle forward and Left/Right
to cycle backward/forward, without opening a picker. Enter still opens the full
picker. Heartbeat toggles use the same keys; Interval opens its numeric control.
Typing filters the list, and the filter is retained after a setting changes.

Open a policy’s **Behaviors** panel to edit specs, enabled states and detection
rules. Each monitoring behavior also exposes its **Action** (Warn or Stop) and
warning color. **Judge** is a sibling panel for the model and Call mode;
**Heartbeat** controls when monitoring runs. Selection has the same direct
Behaviors / Judge layout, with its candidate-selection prompt in Judge.
**New behavior** shows the complete configuration before creation; edits stay in
an unsaved draft until **Create behavior**. Specs use the multiline document
editor (`/save` or the configured save binding; Escape cancels the text edit).
Leaving the new-behavior panel discards its unsaved draft.

The **Detection rule** determines whether a behavior is flagged: **Most likely**
requires estimated probability above 50%; **Threshold** uses your chosen cutoff.
The behavior’s **Action** determines what follows a detection: warn or stop.
Actions are stored with the policy, alongside its criteria and judge settings.
Disabling a behavior skips it while retaining its settings.

Long specs, criteria and prompts use the full document editor. Text wraps and
scrolls with the cursor; use arrows, Page Up/Page Down or the mouse wheel to
navigate. The heading shows the current line and total lines. New evaluation behaviors
ask for a name first, then open this editor for criteria. Saving preserves the
complete multiline text, including content outside the visible window.

The local monitoring **Model alias** is the request identifier sent to OpenJev.
`openjev-latest` routes to DiffusionGemma in Carla’s managed worker; it is not a
second model. The Behaviors row counts enabled Warn/Stop rules and disabled Off rules.
