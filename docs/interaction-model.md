# Interaction model

Carla's actions operate on saved objects and explicit selections. Tabs expose
those objects; they do not redefine the commands. Commands, buttons and keyboard
bindings should resolve the same target and invoke the same operation.

## Actions

| Command | Intent |
| --- | --- |
| `/add` | Add material to the current collection. |
| `/remove` | Remove selected membership or content, showing the consequence. |
| `/branch` | Make an independent alternative without generating. Preserve ancestry. |
| `/continue` | Advance the existing item or selected set, retaining its history. |
| `/loom N` | Generate N alternative futures of the selected item or set. |
| `/eval` | Judge exact saved versions using the active or named evaluation. |
| `/export` | Write a frozen copy of selected saved items, or the current collection. |

The shared command palette contains `/config`, `/policy`, `/stop` and `/help`.
Stage navigation uses `/library`, `/branches`, `/anthology`, `/simulator` and
`/evaluate`. Editor controls, selection controls, notes and inspection remain
available through their owning interfaces. Compatibility aliases need not be
separate entries in the palette.

`/config` owns execution defaults, model selection, workspace selection and key
bindings. `/policy` owns monitoring, selection and evaluation definitions. A model
flag overrides the default for one operation; it does not change future defaults.

## Focus, selection and scope

Arrows preview; Space selects. Enter opens an object and targets it. Moving to the
command bar preserves that context. A visible selection wins over the preview.
In Simulator, clearing selection explicitly returns to fresh generation; merely
hovering a previous conversation must not make it a generation target.

A conversation set **contains** its members. A document parent represents
**ancestry**. Anthology **references** kept versions. Those relationships must not
be treated as the same recursive selection operation. Batch curation can include
descendants explicitly; generation must preserve the selected starting versions.

Library passage selection composes a seed document, following the same document
generation path as Branches. Anthology points to kept document versions: generating
from one produces material in Branches which still needs separate curation.

## Continue, Loom and Branch

For one selected set A containing four conversations:

```text
/continue   A → A with four advanced conversations (prior revisions retained)
/loom       A → B with four alternative continuations
/loom 3     A → B, C, D; each contains four alternative continuations
/branch     A → B with four copied conversations; no generation
```

For a single item, the same rules apply at item scope. Loom 1 and Continue can
produce the same amount of text; they differ in identity and ancestry. A document
continuation from an old revision or an earlier cursor position preserves the
existing future by branching instead of rewriting it. Exact saved versions remain
available to Anthology and evaluations.

A subset advances only those members. Loom preserves the subset as an alternative
set; it never concatenates conversations into one model prompt. Selection policies
between Loom loops select complete alternatives at that grouping level, rather
than silently assembling winning children from different alternatives.

## Current scope and limitations

A document continuation from the list uses the complete saved text. Automatic
scrolling to the first change is a reading aid, not a generation boundary. Moving
the cursor explicitly in the document reader selects a prefix for generation.

The document backend currently represents one ordered set of saved versions.
It does not yet represent a nested set of document sets: selecting members from
multiple sets combines them into a flat selection. Simulator preserves source-run
groups when targeting conversations from multiple runs. Arbitrary recursive
nesting is not implemented in either domain.

Document ancestry and set membership are separate relationships. The current
parent checkbox still selects descendants for batch curation, so inspect the
checked versions before generating. Continue rejects a selection containing
multiple revisions of the same logical document. These selection limitations
remain under review; the action rules above describe the intended contract.

## Inputs and overrides

```text
/continue --tokens 512
/loom 4 --tokens 512
/continue --visitor "What matters to you?" --model base-alias
/loom 3 --visitor "What matters to you?" --turns 2 --loops 4
```

`--visitor` supplies the next visitor message before generation, once per selected
conversation. The preview states that scope. An existing unanswered visitor turn
is a conflict: it is not silently replaced. Without a supplied message, an already
pending visitor turn receives the next character response. The flag applies only
to Simulator. Long visitor text can be entered through the conversation editor.

`--model` selects the document/character generator for this operation;
`--visitor-model` selects the simulated visitor. Saved traces record effective
settings and models. `--tokens` caps each generation; it is not a target length.
`--turns` controls new character replies in Simulator. `--loops` repeats Loom's
generate-and-select cycle; it is not a Continue option.

## Evidence and persistence

Continue must retain original text, turns, prompts, settings and model identities.
Judgments and training marks apply to the frozen content they evaluated. Advancing
an item does not transfer a passing label to its new content. Branch and Loom
preserve ancestry; editing does not rewrite downstream replies based on old text.

Exports contain exact text, structured messages, metadata and provenance. They do
not select a training framework, tokenization scheme, loss mask or preference
objective. They do not train a model or upload data. An export is not a workspace
backup or a supported workspace restore format.

## Ownership and verification

Go owns focus, selection, editors, command completion and rendering. Python owns
validated operations, grouping, revisions, inference, persistence and evidence.
The same operation plan should explain the request before it runs. One session
owns an active job with bounded model concurrency; set structure survives queueing,
streaming, stopping and recovery.

Acceptance covers single items, subsets and parents; clear-to-fresh behavior;
Continue versus Loom identity; visitor/model overrides; policies over complete
sets; frozen judgments; command/editor navigation; and restart with partial work.
