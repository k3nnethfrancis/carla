# Interaction model

Carla is a workspace for exploring and shaping model-generated material. Its
commands are actions on that material, not a second navigation system to memorize.

```text
Where am I?       What am I acting on?       What will happen?
view + focus  →   explicit selection    →    action + settings → result
   preview        item / parent / set        visible scope       selected
```

## The contract

**Focus** is the object under the cursor and the pane receiving keys. Arrows move
focus and change the preview. **Selection** is the checked target of an operation.
The two must remain distinct: looking at another item must not redirect a batch.
**Editing** is a separate mode, with a draft and explicit Save/Cancel.

Parents are useful operation targets. Selecting a parent means its children;
selecting a child from that scope narrows it to that child. Additional Space presses
build a subset. Enter opens an object and explicitly targets it. Returning outward
with Escape changes the view, not the selected set. Clear removes the target.

The selected object, its cardinality and the operation determine capability:

| Object / scope | Generate | Inspect or edit | Evaluate |
|---|---|---|---|
| Source / document prefix | Continue into new versions | Inspect source; edit a version | Saved document versions |
| Single conversation | Extend, or generate alternatives | Open transcript; edit into a fork | Full saved transcript |
| Loom parent | Extend each child independently | Open comparison grid | Every child transcript |
| Explicit conversation subset | Extend just those members | Preview without changing set | Those transcripts |
| Evaluation collection | No generation operation | Inspect judgments and notes | Run configured judges |

This is an interaction contract, not a claim that every action already accepts
every kind of set. Document generation still uses one active prefix; multi-document
generation is not implemented. Fork and message editing require one conversation.
The Simulator now supports parent and sibling-subset generation and evaluation.

## Code ownership

- `tui/simulation_selection.go` owns explicit Simulator selection and resolves it
  into concrete conversation references. Rendering, command requests and evaluation
  targets use it. No inference or persistence occurs there.
- `tui/command.go`, `command_guidance.go` and `loom.go` expose actions, describe their
  flags and dispatch the resolved target. Aliases reach the same operation.
- `src/character_lab/simulator_commands.py` validates references, settings and scope.
  `simulator.py` freezes each selected history and generates its descendants.
- Domain state and exact ancestry live in Python. The UI never fabricates a merged
  transcript or silently chooses one conversation as the source for a group.

A future shared capability registry should describe **action × object type ×
cardinality × mode**, including its label and constraints. It should feed command
availability, buttons, keybindings and help rather than duplicate execution logic.
Extend the current resolvers when a concrete workflow needs it; avoid a generic
framework that claims unsupported combinations work.

## Acceptance journey

1. Run `/loom 4` in Simulator with nothing checked.
2. The resulting parent is selected; `/continue` extends all four histories.
3. Enter its grid, arrow to a child and Space-select it. Only that child is checked.
4. Arrow to another child and Space-select it. Both are targets; previews are not.
5. `/loom` advances those two using current turns and sampling settings. `/eval`
   addresses the same two if used instead.
6. `/clear` makes the next `/loom` fresh. Nothing is deleted.

Command previews must describe the selected scope before execution. Tests cover
arrows versus checks, parent-to-subset transitions, alias dispatch, independent
history preservation and stale targets; terminal QA covers the same journey.
