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
| `/eval [name]` | Run the active or named policy on selected saved content. |
| `/export` | Export selected saved items, or the current collection when none are selected. |

The palette shows relevant actions. Compatibility names are searchable without
creating duplicate entries. Editor operations remain accessible through bindings
and contextual controls; old editing commands continue to work.

## Navigation and controls

`/library`, `/branches`, `/anthology`, `/simulator`, `/evaluate` open the five stages.
The shared controls are:

- `/config`: execution settings, models, workspace selection and keybindings.
- `/policy`: named Monitoring, Selection and Evals policies, with judges and actions.
- `/behaviors`: reusable behavior specs for this workspace.
- `/stop`: cancel the active operation, retaining partial output.
- `/help`: command descriptions and arguments.
- `/rename`: name a focused document or individual conversation; optionally update descendant ancestry names.

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
| `--eval "name"` | Continue, Loom | Assess completed outputs with a named policy |
| `--selection on\|off` | Loom; Off also accepted by Continue | Override selection for this run. On requires 2+ alternatives and an explicit `--loops N`. |
| `--monitoring on\|off` | Continue, Loom | Override monitoring for this run using the configured provider and behavior rules. |
| `--loops N` | Continue, Loom | Repeat generation; with selection enabled, judge each batch and continue from the winner. |

Persistent defaults live in `/config`. Branches puts **Tokens** first, with model
selection and **Advanced settings** for sampling and context. Simulator puts
**Turns**, **Character tokens** and **Visitor tokens** first; speaker sampling and
**Prompts** open submenus. Turns counts additional character replies per loop;
tokens cap each individual completion, not the whole Loom. Flags override one operation and are saved
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
Selection On in `/policy` chooses a complete alternative after each batch when
`--loops` is supplied, and splits again from the winner if another loop remains.
One-output loops always advance directly. An explicit `--selection on` requires
2+ alternatives and an explicit `--loops`; invalid combinations fail before generation.
`--loops 1` generates and selects once without generating another batch.

```text
/loom 4 --selection on --loops 1
/loom 4 --selection on --monitoring off --loops 3
/continue --monitoring on
```

Both flags override only this run. Document Continue/Loom defaults to monitoring
and selection Off, even if a Simulator policy is On. Opt in with `--monitoring on`
or `--selection on`; evaluations run only with `--eval "policy"`.
Simulator inherits saved policy settings when flags are omitted;
`--selection off` bypasses selection even when saved On. `--monitoring on` uses
the configured provider (or the last explicitly chosen provider when saved Off).
If none has been configured, Carla asks you to choose one in `/policy` first.
Required provider setup still applies. No provider is chosen implicitly.
A selection policy never combines individual winners from different sets. All
candidates and decisions remain available. Monitoring is a separate optional check during generation;
only explicitly configured Stop actions terminate flagged work.

## Evaluation

Evaluate has Data, Policies and Results. Its Policies view and `/policy` → Evals
use the same named policies. `/policy` always shows all three policy categories;
`/config` configures the current view. Add behaviors and judge settings to a policy,
add saved documents/conversations to Data, then run the policy on those items.
Adding existing evidence does not run another judge.

```text
/eval
/eval "Voice"
/eval "Voice" --train-on-pass true
/loom 3 --tokens 512 --eval "Voice"
```

`/eval` on selected Branches/Anthology/Simulator material runs the active policy
or the named policy. In Evaluate, `/eval` opens the same setup as **+ New run**,
with the focused dataset or selected items preselected. Start runs it; previously
evaluated items can be assessed again. If configuration
is missing, Carla explains what to set up. `--train-on-pass true` marks an item only
when every enabled behavior completes and passes. The policy’s Actions set the
default (initially off); an explicit true or false overrides it for that run. Manual training marks
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
