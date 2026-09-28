# Carla command system

**Command reference · unified command system**

This is the command contract for the unified Loom interface. Legacy command names resolve to the canonical actions below.

## The organizing idea

Carla has four views: **Library, Branches, Anthology and Simulator**. Commands operate on the selected object. The view supplies context, rather than introducing a different command vocabulary.

Library supplies source passages. Branches holds document versions and their ancestry. Anthology is the kept subset of those versions. Simulator holds conversations and batches of alternative conversations.

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
| `/config` | Edit saved settings and policies relevant to the current workflow. |
| `/fork` | Create a new version of the selected document or conversation without generation. |
| `/edit` | Change existing document text or a conversation message; saving creates a new version. |
| `/remove` | Remove the selected item from its current context, with explicit consequences. |

`/branch` is an alias for `/fork`. `/delete` is an alias for `/remove`. Typing an alias should surface the canonical action, not a duplicate menu entry.

The Branches tab keeps its name. “Fork” names the action; “Branches” names the resulting tree.

## Loom: configure one set, then repeat

```text
/loom [alternatives] [--tokens N] [--turns N] [--loops N]
       one set       generation settings       repetition
```

Examples and autocomplete place `--loops` last. The parser accepts flags in any order.

| Parameter | Meaning | Availability |
|---|---|---|
| Alternatives | Number of alternatives generated from the same starting point. | All views |
| `--tokens N` | Maximum new tokens per generation; not a required length or total conversation budget. | All views |
| `--turns N` | New Character replies per conversation alternative per loop, with Visitor messages as needed. | Simulator only |
| `--loops N` | Total generate-and-select cycles, including the first cycle. | All views |

**Bare `/loom` is the small, predictable action:** one alternative, one loop, and in Simulator one new Character reply. Model, sampling, token ceilings and monitoring settings still come from configuration. Explicit parameters expand the run.

Do not silently ignore incompatible parameters. `--turns` outside Simulator should explain that turns apply only to conversations. There are no mode flags.

### Documents

```text
/loom
    One continuation from the selected passage or document prefix.

/loom 4 --tokens 512
    Four alternatives of that same prefix, each up to 512 new tokens.

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

/loom 3 --tokens 512 --turns 2 --loops 4
    Three alternatives per loop, two Character replies per alternative,
    repeated for four loops, advancing one selected path.
```

If all loops complete, the last example produces twelve candidate extensions. The final path gains eight Character replies, plus the required Visitor messages. The token ceiling applies to individual generations, not the whole eight-reply path. An explicit `--tokens` overrides both Character and Visitor for this run; omitting it preserves their individual saved ceilings.

If the conversation ends with a Visitor message, Loom generates the Character response directly. It must not invent another Visitor message first. If the conversation needs a Visitor message before the next Character reply, the configured Visitor supplies it. Fresh runs use the configured opening.

## Selection determines the input

A batch is a collection of alternatives, not itself a conversation.

| Selected context | `/loom` | `/loom 4` |
|---|---|---|
| One document version | One continuation | Four continuations of that version |
| One conversation | One new Character reply | Four alternative extensions of that conversation |
| Completed conversation batch | Open the grid and ask which conversation to use | Open the grid and ask which conversation to use |
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
  Generation     models, sampling, token ceilings
  Selection      criteria and advancement rules (counts use command arguments)
  Policies
    Selection    evaluator, criteria, labels, advancement rules
    Monitoring   evaluator, behavior specs, thresholds, actions
  Conversation   source documents, speakers, opening [Simulator]
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

Monitoring can accompany a single-loop or multi-loop run. Enabling it never enables automatic exploration. Selection and monitoring retain separate evaluator settings, prompts and specs even though both are configured through `/config`.

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
| Simulator | `/loom`, `/config`, `/fork`, `/visitor` | Generate conversations, configure them, fork one, or add a Visitor message. |
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

The interface replaces separate user-facing `/continue`, `/grow`, `/run` and configuration/policy commands with `/loom` and `/config`. Compatibility names resolve to the canonical action; see the alias rules below.

## Decisions and current limits

- The local selector reviews every candidate and selects one classified `explore`, or none. Multiple eligible candidates are resolved by the selector’s explicit choice and rationale. No selection ends exploration with `no_selection`; invalid evidence or provider failure fails the run and retains outputs. There is no silent random fallback.
- Selection happens after a completed batch. A selector uses a separate resident model after generation is unloaded. Externally managed generators cannot be unloaded by Carla; stop that server before using selection loops.
- Monitoring settings are currently shared between document and conversation workflows. It remains optional and sends the material being checked to OpenRouter when enabled. Document monitor evidence is persisted and inspectable; the conversation-specific blink UI is not reused for document tiles.
- `/continue`, `/generate`, `/run`, `/simulate` and `/grow` are compatibility names for `/loom`. Their positional argument follows Loom’s alternatives count. Use `--tokens` explicitly for an output ceiling. `/grow` no longer silently enables repeated loops.
- Old settings, sampling and policy command names lead to `/config`. Existing keyboard action IDs remain supported. `/model` remains a direct shortcut.
- Conversation edits fork through the changed message and discard later replies in the fork, preserving the original.
- Explicit multi-selection to continue several conversations is not implemented. Choose one conversation; a count creates alternatives of that conversation.
- No saved batch-preset UI is introduced. Bare Loom always creates one alternative in one loop and, in Simulator, one Character reply.
- Training is not implemented. Keeping and exporting remain explicit human curation actions.
