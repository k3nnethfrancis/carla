# Using Carla

Carla's five tabs cover source selection, branching, curation, conversation
sampling and evaluation. Start with one small experiment before scaling up.
See the [README](../README.md#quick-start) for installation and model setup.

## First experiment

1. Open `carla --workspace first-experiment`. Choose a local base GGUF, download
   a supported model, or skip setup to explore the Library. Generation requires
   a compatible `llama-server`; Carla does not install it.
2. In **Library**, select a document/passage with Space. Enter brings selected
   material into Branches. `/add` adds a local text/Markdown file to the shared
   library; importing does not select it in every workspace.
3. In **Branches**, choose a version, position the document cursor and run
   `/loom 3 --tokens 512`. This creates three continuations of the same prefix.
   Outputs stream into a comparison grid. Enter opens one; `/grid` returns.
   `/inspect` shows its actual input, settings and provenance.
4. Select promising versions with Space and `/add`. They appear in **Anthology**,
   a curated view of the branch tree. There is no required anthology size; choose
   coherent material that fits the context budget and your experiment.
5. Open **Simulator**, then `/config`. Explicitly select anthology versions,
   Character/Visitor models, templates, opening and sampling. Keeping documents
   does not automatically include all of them in every simulation.
6. Use `/clear`, then `/loom 3 --turns 2 --tokens 512` for three fresh conversations.
   Each adds two Character replies, with Visitor messages between them. Open a
   conversation to inspect its text, prompts, settings and policy evidence.
7. In **Evaluate → Data**, add saved conversations to a collection. In
   **Policies**, add behaviors and judge settings, and choose the active policy. Run `/eval`
   on the data and inspect **Runs**. Review evidence, mark desired training
   items with the collection controls and use `/export` to save the selected items or collection. The [evaluation guide](evaluations.md)
   covers reuse, reruns, pass rules and metadata.

A conversation conditioned on anthology text is an experiment with a base model,
not a newly trained model. Carla currently ends at dataset curation/export.

## Browse while work runs

Arrows, the mouse wheel and sidebar clicks focus an item and refresh its preview.
Space selects command targets independently of focus, including during generation.
Enter opens the focused item; if its preview is still loading, that open is retained
unless you navigate away. In Simulator, opening a conversation also selects it.

Background additions preserve the focused item's identity and do not move your
grid page. `/active` explicitly follows the running output; browsing or moving in
the grid pauses that following. You can scroll up to read earlier text while
output continues. Generation and destructive commands still honor operation locks.

## Inspect a generation or judgment

`/inspect` opens tabs inside the document pane. **Left/Right** switches between
Overview, Generation, Monitoring, Selection, Evaluations and Raw; **Up/Down**
scrolls the current section. Each tab keeps its own scroll position. Tabs without
saved evidence are omitted, with missing evidence listed in Overview. Missing
historical information does not imply a policy was off.

**Enter** opens the raw evidence list. Saved records and token-event streams are
separate entries. **Escape** returns to the same tab and scroll position, then to
the original view. Saved policy evidence is historical; changing a policy now
does not change those results. Selection retries remain available in the evidence
list and require confirmation.

Monitoring results are metadata, never part of the document or conversation text.
A title such as **complete · monitoring error** means generation completed but a
monitoring check failed. Use Inspect for the cause and any successful checks.
A saved warning or error remains attached to that generation as evidence; it does
not mean a judge is still running.

Document continuations opt into judging with `--monitoring on`, `--selection on`
(with alternatives and explicit loops), or `--eval "policy name"`. The saved
Simulator policy switches do not enable judging for document continuations.

## Navigate and edit

- Tab/Shift+Tab move between panes and the command bar; arrows move within a pane.
  Escape moves outward one level. `/` opens commands. `/config` includes keybindings.
- Command suggestions show arguments for the highlighted command. `/help` → Enter
  opens scrollable details. Use arrows, Page Up/Page Down or the mouse wheel.
- Space selects items; entering a document opens it for navigation. Use `/edit`
  to write, `/save` to commit a new version, and `/cancel` or Escape to discard
  the draft. `/branch` creates a version without generating.
- `/notes` opens document notes or an evaluation-item note. Notes are metadata;
  they are not inserted into generation prompts. `/review` records a document
  verdict/quoted passage separately from automated judgments.
- `/rename` changes a document title. `/find` filters the current list. `/active`
  returns to a running generation. `/stop` cancels work while retaining partial text.

In Simulator, Space selects a Loom parent (all conversations) or toggles children.
Selecting a child replaces the parent scope; further children build a subset.
Enter opens and selects a parent or one conversation. Arrows browse without
changing the checked set. `/continue` advances those existing conversations using
configured turns; `--turns` overrides them. `/loom N` creates N alternative futures:
for a selected parent containing four conversations, `/loom 3` creates three sets
of four. A finished Loom selects its outputs so you can continue immediately.
Clear selection to make the next Loom fresh. `/eval` shares the same selected
scope. Use `/continue --visitor "Your message"` to supply the next visitor turn,
or `/loom 4 --visitor "Your message"` to explore four alternative responses.


Library Enter opens a new source root in Branches. Anthology Branch/Continue
returns new output to Branches without keeping it; Anthology Loom starts Simulator.
Library and Evaluate do not run generation commands.

`/remove` is contextual: deselect Library sources, remove Anthology membership,
confirm branch/subtree deletion, or remove Evaluate collection membership. Branch
removal displays consequences and writes a recovery snapshot. It is not a
universal filesystem-delete command. See [commands](commands.md) for exact scope.

## Repeat and compare

One-output Loom continues the selected target. A larger count splits alternatives.
`--loops N` repeats continuation on those outputs. Selection defaults to Off;
enable it under `/policy` for Simulator, or pass `--selection on` for document
Looms, to select a whole alternative and split again between loops. Selection
requires at least two alternatives and explicit `--loops N`; one loop selects once.
The selector may choose none, ending exploration with its explanation retained.
Monitoring is separately optional; only enabled Stop actions stop flagged output.
A token ceiling limits an individual generation, not the number of later turns.

Use an explicit run name/notes, frozen source versions and the same evaluation
criteria when comparing changed prompts or settings. Add newly generated versions
as new evaluation items. Rerunning an old item judges its frozen text; it does not
regenerate the conversation under today's settings.

## Data, backups and updates

By default, application data is in `~/.local/share/character-lab`, separate from
this Git checkout. `CARLA_DATA_DIR` changes that root. Workspaces have their own
project state, journal, command history and view state. The shared library,
model registry, keybindings and credentials live at application level.

- Reinstalling/rebuilding the same checkout does not remove your workspace data.
  Moving the checkout requires rerunning `make install` to update its launcher.
- Back up a workspace's **whole directory while Carla is closed**. Include the
  application-level library/configuration separately if you need to reproduce
  the full setup. Local model paths still need to resolve on the new machine.
- `project.json` and the stream journal preserve saved branches and partial work.
  Crash recovery is not a guarantee against disk failure or power loss.
- Startup restores the last tab/document with the command bar focused. It does
  not restore editing mode or checked Loom targets. Branches and Simulator start
  collapsed, revealing only the ancestors of the saved item; without a valid saved
  item, their trees stay collapsed. Exit recovery drafts are
  separate from committed versions; inspect them before deleting recovery data.
- `/export` writes selected items or the current collection under `exports/`, with
  a versioned JSON manifest and text reading copies. Conversations retain structured
  turns; evaluation exports include judgments and training metadata. Exports are
  not a replacement for a full workspace backup.
- Raw prompts, outputs, source text and judgments can contain private material.
  Inspect exports before sharing. Credentials are stored separately, not in traces.

To update a source installation, save/exit Carla, pull the intended branch and
run `make install` again. Keep your own uncommitted code changes safe when pulling.
Use `carla --version` with bug reports.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Generation cannot start | GGUF exists; `llama-server` is on PATH and supports the flags in [model configuration](configuration.md#models). Check workspace `model-server.log`. |
| Out of memory / very slow batches | Lower configured context/output budgets or choose a smaller model. `Max` can reserve all remaining context and serialize work. Carla does not silently shrink budgets. |
| Loom selection cannot start | Configure the selection judge and behaviors, and use two or more alternatives with explicit `--loops N`. Selection Off continues all outputs and needs no selector. |
| Wrong conversation is continued | Inspect the checked target; `/clear` starts fresh. Hover is not selection. |
| Evaluation opens but cannot run | Configure an Evals policy’s judge and enabled behaviors. In Branches or Simulator, select saved content and use `/eval [policy]`. In Evaluate, choose a dataset and policy in New run. |
| Local judge unavailable | Complete the one-time cache setup with `scripts/local-judge.sh`, then select DiffusionGemma under `/policy`. Carla manages the local server automatically. See [local judge setup](local-judge.md). |
| Jev authentication/provider error | Check saved credential versus launching environment, provider access and quota. Saving a key does not validate it. |
| Workspace already open | Close the owning Carla process; each workspace permits one writer. Do not edit its JSON while running. |
| Terminal layout unavailable | Resize to at least 60×18; use a UTF-8 terminal. |

For reproducible bugs, include version, OS/terminal, command, relevant settings
and a small non-sensitive example in [GitHub Issues](https://github.com/k3nnethfrancis/carla/issues).
