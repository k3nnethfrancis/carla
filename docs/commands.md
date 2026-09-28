# Carla command system

**Command reference · unified command system**

This is the command contract for the unified Loom interface. Legacy command names resolve to the canonical actions below.

## The organizing idea

Carla has five views: **Library, Branches, Anthology, Simulator and Evaluate**. Commands operate on the selected object. The view supplies context, rather than introducing a different command vocabulary.

Library supplies source passages. Branches holds document versions and their ancestry. Anthology is the kept subset of those versions. Simulator holds conversations and batches of alternative conversations. Evaluate holds judged material and training selections.

There are two generation workflows behind one command:

```text
Library ─── selected source ──────┐
Branches ── selected prefix ─────┼── /loom ── document continuations ── Branches
Anthology ─ selected kept prefix ┘

Simulator ─ opening or transcript ── /loom ── conversation extensions ── Simulator
```

Library and Anthology call the same document operation as Branches, then open its results in Branches. They do not implement separate generation systems. New document versions start unkept; the original source or kept version remains unchanged.

## The central commands

| Command | Meaning |
|---|---|
| `/loom` | Generate from the selected starting point. |
| `/config` | Edit generation settings relevant to the current workflow. |
| `/policy` | Configure monitoring, selection and reusable judge configurations. |
| `/eval` | Run active/named evaluation on selected saved material. |
| `/evaluations` | Manage evaluation collections, results and training selections. |
| `/fork` | Create a new version of the selected document or conversation without generation. |
| `/edit` | Change existing document text or a conversation message; saving creates a new version. |
| `/remove` | Remove the selected item from its current context, with explicit consequences. |

`/branch` is an alias for `/fork`. `/delete` is an alias for `/remove`. Typing an alias should surface the canonical action, not a duplicate menu entry.

The Branches tab keeps its name. “Fork” names the action; “Branches” names the resulting tree.

## Loom: configure one set, then repeat

```text
/loom [alternatives] [--tokens N|Max] [--turns N] [--msg "opening"] [--eval "name"] [--loops N]
       one set       generation settings / evaluation                       repetition
```

Examples and autocomplete place `--loops` last. The parser accepts flags in any order, including `--flag=value`. Count also accepts `--count N` or `-n N`.

While typing `/lo`, the highlighted `/loom` completion already shows its arguments. Arrow keys change the highlighted command and its preview. Arguments wrap to remain visible; `/help` provides their full descriptions; Enter opens scrollable details and Escape returns to the list. Quotes preserve spaces in evaluation names and messages.

| Parameter | Meaning | Availability |
|---|---|---|
| Alternatives | Number of alternatives generated from the same starting point. | Library, Branches, Anthology, Simulator |
| `--tokens N\|Max` | Maximum new tokens per generation; not a required length or total conversation budget. | Library, Branches, Anthology, Simulator |
| `--turns N` | New Character replies per conversation alternative per loop, with Visitor messages as needed. | Simulator only |
| `--msg "text"` / `--message "text"` | Override the first Visitor message for this fresh run; preserves the saved opener. | Simulator only, no conversation selected |
| `--eval "name"` | Judge completed outputs using that named evaluation after generation; does not change generation prompts. | Library, Branches, Anthology, Simulator |
| `--loops N` | Total generate-and-select cycles, including the first cycle. | Library, Branches, Anthology, Simulator |

**Bare `/loom` is the small, predictable action:** one alternative, one loop, and in Simulator one new Character reply. Model, sampling, token ceilings and monitoring settings still come from configuration. Explicit parameters expand the run.

`--turns` and `--msg` outside Simulator are rejected with an explanation. There are no mode flags. `--loops` greater than one requires a configured selection policy; a single loop does not select a winner.

### Documents

```text
/loom
    One continuation from the selected passage or document prefix.

/loom 4 --tokens 512
    Four alternatives of that same prefix, each up to 512 new tokens.

/loom 4 --tokens 512 --eval "Voice"
    Generate four alternatives, then run the Voice evaluation on their saved text.

/loom 3 --tokens 512 --loops 4
    Three alternatives per loop, four loops total.
    Selection advances one eligible result between loops.
```

If all four loops complete, the last example creates twelve continuations. The advancing path contains four extensions, each with its own token ceiling. Unselected alternatives remain available. Selection does not automatically keep documents in Anthology.

### Conversations

```text
/loom
    One new Character reply from the selected conversation or fresh setup.

/loom 4
    Four alternative next replies from the same starting conversation.

/loom 3 --tokens 512 --turns 2
    Three alternative conversation extensions, each adding two Character replies.

/loom 3 --tokens 512 --turns 2 --eval "Voice" --loops 4
    Three alternatives per loop, two Character replies per alternative,
    repeated for four loops, advancing one selected path.
    Then judge completed conversation outputs with Voice.
```

If all loops complete, the last example produces twelve candidate extensions. The final path gains eight Character replies, plus the required Visitor messages. The token ceiling applies to individual generations, not the whole eight-reply path. An explicit `--tokens` overrides both Character and Visitor for this run; omitting it preserves their individual saved ceilings.

If the conversation ends with a Visitor message, Loom generates the Character response directly. It must not invent another Visitor message first. If the conversation needs a Visitor message before the next Character reply, the configured Visitor supplies it. Fresh runs use the configured opening unless `--msg` or `--message` supplies a quoted opener. For example, `/loom 4 --msg "What does a path remember?" --turns 2 --tokens 512` starts four conversations with that message. Single or double quotes preserve spaces; nothing is expanded or executed as shell code. An opener with a selected conversation is rejected: use `/clear` for a fresh run, or `/visitor` to add a message to an existing conversation.

## Selection determines the input

A batch is a collection of alternatives, not itself a conversation. In Simulator, highlighting previews a target but does not select it for Loom. **Space** selects or deselects one conversation; **Enter** selects and opens it, including from the grid. Choosing a different conversation replaces the prior checkmark. `/clear` clears the target without deleting anything. The checked conversation stays the Loom target while browsing other items. Starting a Loom consumes that selection; displaying its output does not implicitly select a new target.

| Selected context | `/loom` | `/loom 4` |
|---|---|---|
| One document version | One continuation | Four continuations of that version |
| Explicitly checked conversation | One new Character reply | Four alternative extensions of that conversation |
| Browsed conversation or batch, nothing checked | Start one fresh conversation | Start four fresh conversations |
| Fresh Simulator setup | Start one conversation | Start four alternatives from the setup |

Never silently pick a batch winner, continue every member, or reuse the original batch input. Automatic policy selection is part of an explicitly configured multi-loop run.

Continuing several existing conversations is different from making several alternatives of one conversation. Explicit multi-selection is the proposed entry point for that bulk operation, but its command contract is not yet settled.

## Fork, edit and Visitor messages

`/fork` creates a child version without inference. In Anthology it opens the new, unkept version in Branches. In Simulator it forks the selected conversation. A batch must first resolve to one conversation.

`/edit` changes existing text. Saving preserves the original and creates a new version. It appears once in Shared, with context-specific behavior, rather than being duplicated in Simulator’s command list.

`/visitor` is Simulator-specific: write a new Visitor message into a fork of the selected conversation. Then `/loom` generates the next Character reply. Adding the message does not itself trigger inference.

```text
Select conversation → /visitor → write message → save fork → /loom
Select conversation → /edit    → revise message → save new version
Select conversation → /fork    → new version, no generated text
```

## Configuration and policies

`/config` is one entry point into contextual configuration.

**Library, Branches and Anthology share document Loom settings.** They are views into the same configuration, not three copies. Alternatives, turns and loops are explicit run arguments; their former default controls are hidden to keep bare Loom predictable. Simulator exposes conversation settings alongside the common Loom controls.

```text
/config
  Generation     models, prompts, sampling and source selection

/policy
  Monitoring     conditions, thresholds and warn/stop actions
  Selection      evaluator, criteria and candidate advancement
  Evaluations    named whole-item judges, criteria and pass rules
```

Configuration edits persist. Explicit command arguments override settings for that run only. Bare Loom retains the one-alternative / one-loop / one-reply behavior defined above. Saved legacy batch fields remain compatible with existing workspaces; use explicit command parameters to run batches.

`/model` remains a shortcut to model selection within this configuration. It must use the same underlying settings.

### Selection: where to explore next

The former Grow role becomes multi-loop Loom:

```text
Generate alternatives → classify against selection specs → route → next loop
```

Selection is classification. The criteria describe which results qualify for further development. An explicit advancement rule chooses among eligible candidates. Classification confidence is not automatically a ranking of quality.

The advancement rule advances **one candidate per loop**, retaining the other alternatives. On the last loop, a verdict can identify a preferred result, but it cannot trigger an extra loop or keep a document automatically.

A multi-loop run needs a configured selection policy. Missing configuration should lead to that configuration before the run begins. Single-loop runs finish for human review without requiring automatic selection.

### Monitoring: what is happening during generation

The current Jev role remains distinct:

```text
Observe output → classify behavior → annotate / warn / explicitly stop
```

Monitoring runs at configured in-generation checkpoints and completion boundaries. Specs name conditions such as looping or spiraling. Each condition has its configured action and threshold. Stop behavior must be explicitly configured; a warning is not a stop.

Monitoring can accompany a single-loop or multi-loop run. Enabling it never enables automatic exploration. Selection and monitoring retain separate evaluator settings, prompts and specs even though both are configured through `/policy`.

## The complete command map

Commands requiring a selected object are available only when that target exists. Shared means reusable across relevant contexts, not that every action is valid on every screen.

| Area | Commands | Purpose |
|---|---|---|
| Library | `/import`, `/add`, `/remove`, `/clear` | Import a source, choose workspace passages, remove passages, clear the workspace source selection. |
| Library | `/loom`, `/config` | Start the shared document workflow and configure it. |
| Branches | `/loom`, `/config`, `/fork` | Generate, configure or copy a version. |
| Branches | `/keep`, `/remove`, `/clear` | Keep versions, delete versions, clear checked rows. |
| Anthology | `/remove`, `/snapshot` | Unkeep versions or export the kept collection with provenance. |
| Anthology | `/loom`, `/config`, `/fork` | Explore kept versions using the document workflow. New versions appear in Branches. |
| Simulator | `/loom`, `/clear`, `/config`, `/fork`, `/visitor` | Generate conversations, configure them, fork one, or add a Visitor message. |
| Shared: navigate | `/library`, `/branches`, `/anthology`, `/simulator`, `/workspace` | Switch views or open/create a workspace. |
| Shared: model | `/model` | Shortcut to the relevant model settings. |
| Shared: edit | `/edit`, `/save`, `/cancel`, `/rename` | Change selected content, save/cancel a draft, or change a document title. |
| Shared: inspect | `/inspect`, `/review`, `/notes` | Inspect inputs/provenance, attach a human review, or manage document notes. |
| Shared: find/view | `/find`, `/grid`, `/active` | Filter items, view a run’s alternatives, or jump to active generation. |
| Shared: help | `/help`, `/keys` | Explain commands and configure keyboard shortcuts. |
| Shared: session | `/stop`, `/restart`, `/exit` | Stop generation, restart Carla, or exit. Preserve existing recovery behavior. |

`/review` is human annotation, not an automated judge. `/snapshot` exports Anthology; it is not a full workspace backup. Rename, notes and review retain their existing target support unless separately extended.

### Remove has explicit contextual consequences

| Context | `/remove` (alias `/delete`) does |
|---|---|
| Library | Removes passages from the workspace source selection. Preserves the Library source and existing branches. |
| Branches | Deletes selected versions and descendants after confirmation, with the existing recovery snapshot behavior. |
| Anthology | Clears kept membership. Preserves the underlying branch. |

The action description must say which consequence applies. A destructive confirmation identifies what will be affected. Simulator conversation removal has not been designed in this command system.

`/clear` clears selection state rather than deleting content. Its existing distinction remains explicit: Library clears chosen source passages; Branches clears checked rows.

## Command discovery and feedback

- Show relevant commands first, using the selected object and current view.
- Explain the action before parameter syntax.
- Match canonical names and aliases to one menu item: typing `/delete` surfaces `/remove`; typing `/branch` surfaces `/fork`.
- Offer shared Loom parameters everywhere, and conversation parameters only in Simulator.
- Display the resolved input and planned workload before execution, without adding a confirmation to every routine generation.
- Present parameters for one set first, followed by `--loops`.
- Preserve arrow-key completion, Enter to execute the selected suggestion, and command history.
- When a collection is selected where an individual is required, open the appropriate chooser instead of guessing.

Example preview:

> Selected conversation · 3 alternatives · 512 tokens per generation · 2 Character replies per alternative · 4 loops total

## Implementation boundaries

Resolve the selected input once, then invoke either document or conversation generation. Share run scheduling, concurrency control, cancellation, output persistence, provenance and grid presentation. Keep the generation-specific logic distinct: document continuation versus alternating conversation turns.

Library and Anthology should only adapt their selection and choose the Branches destination. They must not duplicate the document generation pipeline. Aliases dispatch to the same action. Configuration shortcuts read and write the same settings as `/config`.

The interface replaces separate user-facing `/continue`, `/grow`, `/run` and configuration/policy commands with `/loom`, `/config` and `/policy`. Compatibility names resolve to the canonical action; see the alias rules below.

## Decisions and current limits

- The local selector reviews every candidate and selects one classified `explore`, or none. Multiple eligible candidates are resolved by the selector’s explicit choice and rationale. No selection ends exploration with `no_selection`; invalid evidence or provider failure fails the run and retains outputs. There is no silent random fallback.
- Selection happens after a completed batch. A selector uses a separate resident model after generation is unloaded. Externally managed generators cannot be unloaded by Carla; stop that server before using selection loops.
- Monitoring settings are currently shared between document and conversation workflows. It remains optional and sends the material being checked to OpenRouter when enabled. Document monitor evidence is persisted and inspectable; the conversation-specific blink UI is not reused for document tiles.
- `/continue`, `/generate`, `/run`, `/simulate` and `/grow` are compatibility names for `/loom`. Their positional argument follows Loom’s alternatives count. Use `--tokens` explicitly for an output ceiling. `/grow` no longer silently enables repeated loops.
- Old settings and sampling names lead to `/config`; policy names lead to `/policy`. Existing keyboard action IDs remain supported. `/model` remains a direct shortcut.
- Conversation edits fork through the changed message and discard later replies in the fork, preserving the original.
- Explicit multi-selection to continue several conversations is not implemented. Choose one conversation; a count creates alternatives of that conversation.
- No saved batch-preset UI is introduced. Bare Loom always creates one alternative in one loop and, in Simulator, one Character reply.
- Training is not implemented. Keeping and exporting remain explicit human curation actions.

A sidebar `!` indicates detected policy behavior only while that conversation is running; it is never a selection marker. Completed detections remain in the conversation header at the right and in the per-turn policy evidence.

## Policies and evaluated datasets

`/policy` configures **Monitoring**, **Selection**, and reusable **Judge
configurations**. Monitoring observes generation; selection chooses candidates.
Their results can be attached to an evaluation, retaining their original scope.
An evaluation is a named collection of saved items with configured judges.

**Evaluate** lists your collections. Create one, open it, and use **Configure**
(or `/config`) to choose its name, judges and whether it is the active evaluation.
**Add items / existing judgments** selects documents, conversations or completed
judgments. Adding freezes the source and attaches available policy evidence;
it does not run inference. Open an item with Enter to read its text and judgment
history. Escape returns to the collection, then the collection list.

```text
/evaluations
/config
/eval
/eval Voice
/eval "Character consistency" --train-on-pass true
/loom 3 --tokens 512 --eval Voice
/loom 3 --turns 2 --tokens 512 --eval Voice --loops 4
```

`/eval` runs the active collection's judges on selected material. `/eval name`
chooses a particular collection. In Branches or Anthology, targets are checked
versions, or the highlighted version when none are checked. In Simulator, select
an individual conversation explicitly with Space or Enter; hovering or highlighting
a Loom group is not a target. In Evaluate, checked items or the highlighted item
are rerun; from an action row, Run evaluates pending items. Configuration never
starts a run. `/loom --eval name` judges completed outputs after generation,
using the evaluation's configuration captured when the job started.

Each item freezes its full text and provenance. Every judgment records the judge
revision, request, response and result. Rerunning appends history; edits to the
source or judge do not overwrite it. Changing judge criteria leaves old evidence
visible, but it does not count as a pass for the new revision. An item passes when
all currently configured judge revisions pass. Monitoring observations and
candidate-relative selection decisions remain scoped evidence, not whole-item
grades. Existing completed judgments can be added without another model call.
Older flat results migrate into collections without rerunning or altering them.

A judge uses the configured local instruct model (criteria and editable prompt)
or Jev through OpenRouter (behavior spec and probability threshold). Jev uses
the saved OpenRouter credential or `OPENROUTER_API_KEY`. Its probabilities are
model estimates, not calibrated confidence. Local judges use the policy model;
they must return JSON with boolean `passed`, `reason`, and an exact `evidence`
excerpt. Invalid responses and provider errors remain incomplete, never passes.
Full items are sent without silent truncation; context overflow remains an error.

- Space selects items; Enter opens the result viewer.
- `/keep` marks items for training; `/remove` (alias `/delete`) unmarks them.
  Manual marking is deliberate and separate from judge results.
- `/eval --train-on-pass true` marks an item only when every judge in that run
  passes. It defaults to false; rerunning does not undo prior manual membership.
- `/notes` edits item notes; `/inspect` shows the exact stored item and evidence.
- **Show** filters all, pass, fail, unfinished or training items.
- `/snapshot` exports the collection's training-marked frozen items, judgment
  histories, attached evidence and metadata to a new workspace-local JSONL file.
  Export does not train a model or determine loss masks. Downstream preparation
  must handle repeated content deliberately; snapshot hashes are included.

Generation and judging share the session operation lock. `/stop`, disconnect and
recovery preserve completed work and label unfinished results. Judges run
sequentially with one resident local model. This workflow does not add automatic
judge calibration or a training runner.

## Run status

- **Complete:** the configured work finished.
- **Stopped:** cancellation was handled while Carla was running, including closing
  its terminal. Partial text is saved.
- **Interrupted:** a previous process ended without completing its work; recovery
  preserves the partial output. Older confirmed terminal-disconnect failures may
  also be corrected to this label, preserving their error evidence.
- **Failed:** an actual generation or infrastructure error, such as losing the
  model connection. Inspect the error before retrying.
- **Stopped by policy:** an enabled policy explicitly requested a stop.

Losing the UI connection cancels generation; it is not an inference failure.
Completed conversations and turns retain their status when siblings stop.

### Monitoring timing

In `/policy` → Monitoring → **When to check**, use Enter to toggle **After each
reply** and **During a reply** independently. When during-reply checking is on,
**Check interval** opens an arrow-controlled positive output-token count. Turning
it off preserves that count. Both checks default to on with a 512-token interval;
the separate Monitoring switch still defaults to off.

These controls apply to Character replies and document continuations, not Visitor
messages. Only one check runs at a time; a busy judge coalesces token intervals
rather than queuing requests. Existing zero-interval settings still mean no
mid-reply checks; turning those on starts at 512 tokens. Disabling both timings
means no checks, even if Monitoring is on. Saved traces retain the timing used.

### Enabling monitoring

`/policy` → Monitoring initially shows only **Monitoring · Off**. Select Jev and
complete the masked OpenRouter API-key step before timing, model and behavior
settings appear. If a saved or environment key already exists, setup is already
complete. An older workspace with Jev enabled but no available key shows only the
provider choice and key setup. Turning monitoring off hides its controls while
retaining the configuration. The configured-key row permits replacing a saved key.
