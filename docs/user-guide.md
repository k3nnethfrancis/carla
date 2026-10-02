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
   on the data and inspect **Results**. Review evidence, mark desired training
   items with the collection controls and use `/export` to save the selected items or collection. The [evaluation guide](evaluations.md)
   covers reuse, reruns, pass rules and metadata.

A conversation conditioned on anthology text is an experiment with a base model,
not a newly trained model. Carla currently ends at dataset curation/export.

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
enable it under `/policy` to select a whole alternative and split again between loops.
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
  not restore editing mode or checked Loom targets. Exit recovery drafts are
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
| Multi-loop Loom cannot start | Configure the separate instruct policy model and selection spec. A one-loop Loom does not need a selector. |
| Wrong conversation is continued | Inspect the checked target; `/clear` starts fresh. Hover is not selection. |
| Evaluation opens but cannot run | Add nonempty Data, configure and activate a Policy, then use `/eval`. Local judges currently use the configured policy model. |
| Local judge unavailable | Complete the one-time cache setup with `scripts/local-judge.sh`, then select DiffusionGemma under `/policy`. Carla manages the local server automatically. See [local judge setup](local-judge.md). |
| Jev authentication/provider error | Check saved credential versus launching environment, provider access and quota. Saving a key does not validate it. |
| Workspace already open | Close the owning Carla process; each workspace permits one writer. Do not edit its JSON while running. |
| Terminal layout unavailable | Resize to at least 60×18; use a UTF-8 terminal. |

For reproducible bugs, include version, OS/terminal, command, relevant settings
and a small non-sensitive example in [GitHub Issues](https://github.com/k3nnethfrancis/carla/issues).
