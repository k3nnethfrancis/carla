# Commands

Carla exposes a small action vocabulary. The selected object determines scope;
commands retain their meaning across Library, Branches, Anthology, Simulator and
Evaluate. See [interaction model](interaction-model.md) for identity and ancestry.

Type `/` to open the command bar, type a prefix, use arrows to choose a suggestion
and Enter to execute. The first suggestion is already selected. Its description,
flags and target preview appear beneath the input. Up/Down also recall command
history when you are not selecting a completion. Quoted arguments are parsed as
text, never executed as shell code.

## Work commands

| Command | Meaning |
| --- | --- |
| `/add` | Add material to the current collection. In Library, import a text file; in Branches, keep selected versions in Anthology; in Anthology, choose existing versions to add. Evaluate has collection/item actions. |
| `/remove` | Remove the selected material from its context. In Anthology, unkeep it; in Branches, confirm deletion and its descendant scope. |
| `/branch` | Copy a selected document, conversation or set without inference, retaining ancestry. |
| `/continue` | Advance the existing selected item or set. Prior versions remain saved. |
| `/loom [N]` | Continue the selected target with N=1 (default); N≥2 forks alternatives. Anthology starts Simulator. |
| `/eval [name]` | Run the active or named evaluation on selected saved content. |
| `/export` | Export selected saved items, or the current collection when none are selected. |

The palette shows relevant actions. Compatibility names are searchable without
creating duplicate entries. Editor operations remain accessible through bindings
and contextual controls; old editing commands continue to work.

## Navigation and controls

`/library`, `/branches`, `/anthology`, `/simulator`, `/evaluate` open the five stages.
The shared controls are:

- `/config`: execution settings, models, workspace selection and keybindings.
- `/policy`: monitoring, selection and evaluation definitions.
- `/stop`: cancel the active operation, retaining partial output.
- `/help`: command descriptions and arguments.

Arrows move focus and preview content. Space selects/unselects. Enter opens a
selected object or its actions. Tab/Shift+Tab move between panes; Escape unwinds
one level, cancelling an editor draft before leaving its owner. `/config` exposes
keybindings, including selection clearing, editing, save/cancel and session exit.
Delete invokes Remove in content panels; in text editors it deletes text.

## Continue versus Loom

`/loom` and `/loom 1` act like Continue on an existing target. Library and Evaluate
do not expose generation actions. See the [stage matrix](interaction-model.md#where-actions-work).

| Selected target | `/continue` | `/loom 3` |
| --- | --- | --- |
| One document | New revision of its current logical document | Three alternative versions |
| One conversation | Advance its saved head | Three alternative conversations |
| Set of four conversations | Advance those four in the same run | Three alternative sets of four |
| Explicit subset | Advance only selected members | Three alternative sets of those members |
| No Simulator selection | Select a saved target first | Start three fresh conversations |

After a fresh `/loom 4`, select its parent and use `/continue` to advance all four.
Space on a child narrows the selection to that child; further Space presses can
build a subset. Arrows alone do not replace a checked target. Clearing selection
makes the next Simulator Loom fresh. Opening the command bar preserves scope.

Continue retains logical identity, with older revisions preserved. A document
source, historical revision or earlier cursor prefix becomes a new branch so its
existing future stays intact. Anthology membership and evaluation results remain
attached to exact saved versions; new content is not automatically kept or passed.

Library Enter opens selected passages as a new seed root in Branches. Repeating
this creates a separate root with the same source provenance. Anthology Branch
and Continue create new material in Branches; Anthology Loom starts Simulator
using the selected kept documents. Simulator uses separate conversation histories
and raw completion templates.

## Generation arguments

| Argument | Applies to | Meaning |
| --- | --- | --- |
| positional `N` | Loom | Number of alternative futures of the whole selected shape |
| `--tokens N` or `--tokens Max` | Continue, Loom | Maximum new tokens per model generation, not a requested length |
| `--model alias` | Continue, Loom | Document/character generator for this operation |
| `--visitor-model alias` | Simulator Continue/Loom | Simulated visitor model for this operation |
| `--visitor "text"` | Simulator Continue/Loom | Supply the next visitor message before generation |
| `--turns N` | Simulator Continue/Loom | Number of additional character replies |
| `--eval "name"` | Continue, Loom | Judge completed outputs with a named evaluation |
| `--loops N` | Continue, Loom | Repeat continuation from each output; enabled selection can guide split loops |

Persistent defaults live in `/config`. Flags override one operation and are saved
with its provenance. Model flags do not change workspace defaults. Token caps
apply independently to each speaker; reaching a cap does not stop later turns.
Token ranges are not supported. Conversation-specific flags on documents are
errors rather than silently ignored options.

```text
/continue --tokens 512
/loom 4 --tokens 512
/continue --model base-alias --visitor "What matters to you?"
/loom 3 --tokens 512 --visitor "What does a path remember?" --turns 2
/loom 3 --tokens 512 --eval "Voice" --loops 4
```

Supplied visitor text is sent to every selected conversation; preview the scope
before running. It is inserted once, then subsequent turns use the configured
visitor model. An unanswered visitor message is a conflict: edit it or Continue
without a supplied message to generate its response. Nothing is silently replaced.
`--msg` and `--message` are compatibility spellings of `--visitor`.

Loops do not require a selector. With Selection Off (default), the first loop
creates the requested alternatives and later loops advance those same outputs.
Selection On in `/policy` instead chooses a complete alternative between split
loops and splits again from the winner. One-output loops always advance directly.
A selection policy never combines individual winners from different sets. All
candidates and decisions remain available. Monitoring is a separate optional check during generation;
only explicitly configured Stop actions terminate flagged work.

## Evaluation

`/policy` configures judges; `/config` in Evaluate manages the collection. Configure
criteria, add saved documents/conversations to a named evaluation, then run it.
Adding existing evidence does not run another judge.

```text
/eval
/eval "Voice"
/eval "Voice" --train-on-pass true
/loom 3 --tokens 512 --eval "Voice"
```

`/eval` on selected Branches/Anthology/Simulator material runs the active definition
or the given name. In Evaluate, select collection items to judge. If configuration
is missing, Carla explains what to set up. `--train-on-pass true` marks an item only
when every judge completes and passes. It defaults to false. Manual training marks
and notes remain metadata; changing live content never transfers its old judgment.

Exports preserve training flags but do not filter to training-marked items
implicitly. Select the desired subset first. See [evaluations](evaluations.md) for
judge types, failure handling and evidence.

## Export

Every content view uses the same `/export` action. Explicit selection wins;
otherwise the current collection is exported. In Evaluate, open a collection
first. Exports are local under the workspace's `exports/` directory:

```text
exports/<scope>-<unique-id>/
  manifest.json
  0001.txt
  0002.txt
  ...
```

The versioned manifest contains exact structured records: source passages,
document revisions and needed ancestry, or conversation turns with models,
settings and source documents. Evaluation items include frozen inputs, judgment
history, evidence, notes and training marks. Text files are reading copies.

This is a general-purpose interchange format, not a prepared training dataset.
Choose tokenization, loss masks, deduplication and preference objectives during
training preparation. Export neither uploads data nor trains a model. It is not
a full workspace backup or an import/restore format. Old snapshot files remain
untouched; `/snapshot` is a compatibility name for `/export` in the interface.
