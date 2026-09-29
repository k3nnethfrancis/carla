---
name: tui-design
description: Design, implement, or review Carla's Go Bubble Tea terminal interface, including navigation, document previews, forms, streaming grids, themes and terminal interaction tests.
---

# Carla TUI design

Use this for Carla interface work. Read the relevant code before proposing a
change; preserve the established interaction model unless the task changes it.
This is contributor guidance, never a prompt for a character or selector model.

## Owners

- `tui/`: Bubble Tea **v2**, Bubbles v2 and Lip Gloss v2. Use the APIs in `go.mod`;
  don't copy v1 `View() string` examples into this v2 application.
- `model.go`, `update.go`, `view.go`: UI state, event handling and layout/rendering.
- `command.go`, `bindings.go`, `help.go`: completion, customizable bindings and help.
- `navigation_context.go`, `target.go`: focused context, page restoration and targets.
- `document.go`, `branches.go`, `notes.go`: document navigation and lineage.
- `loom_grid.go`, `conversations.go`, `simulator.go`: streaming comparison and traces.
- `dialog.go`, `model_setup.go`: native forms, nested dialogs and onboarding.
- Python `src/character_lab/`: domain data, persistence, inference and jobs. Go sends
  commands and consumes structured events; it does not own scientific decisions.

Paths above are relative to the repository root. See
[architecture](../../docs/architecture.md) for the boundary and protocol.
Use existing components and handlers; avoid a parallel navigation or settings system.
Keep effects in commands/update handling and rendering free of file/network writes.

## Interaction contract

- The main places are Library, Branches, Anthology, Simulator and Evaluate. Each pane has
  an explicit focus and selected target. Opening a document must not accidentally
  operate on the item that was selected on another page.
- Arrows move within the focused area; Tab/Shift+Tab move forward/backward between
  areas. Enter activates or opens. Space selects collection items; selecting a
  set parent includes its contained members; document rows select exact versions.
  Ancestry is not generation scope; deletion previews descendants separately. A selected mark
  must not shift other rows.
- Escape unwinds one level. In Simulator: conversation → its grid → conversation
  list → tabs. Editing Escape cancels the draft before broader navigation.
- `/` opens the command bar, including from document navigation/editing. Preserve
  literal slashes in path/URL forms. Text-entry focus must not invoke letter hotkeys.
- Continue and Loom1 advance items; Loom2+ creates alternative futures; Branch copies
  without inference. Preserve selected set shape and immutable evaluated versions.
  `/add`, `/remove` and `/export` act on the current collection. The default palette
  stays small; legacy shortcuts remain searchable without duplicate primary rows.
- Commands and visible controls use the same handlers. Rank page-relevant commands
  first, show purpose before argument syntax, and name counts for the context
  (branches versus conversations). The top completion is already selected.
- `/config` exposes editable keybindings (`/keys` remains a compatibility shortcut).
  Display configured key names such as `CTRL+ENTER`,
  not hard-coded defaults or caret notation. Avoid function keys and Command-key
  requirements; terminals do not reliably distinguish those combinations.
- Keep essential controls discoverable without remembering commands. Use explicit
  action labels and visible focus/selection styling, rather than a `>` marker alone.
- Browse and edit are distinct. Preserve draft text and cursor across commands;
  only explicit save commits changes. Preview a selected version at its own first
  change, not an inherited earlier generation. Notes remain separate from text.

## Visual contract

- Start with spacing, alignment and restrained borders in the existing monochrome
  theme. Use the same palette and component sizes across main screens and setup.
- Source, AI continuation and human edits have distinct styles. AI text is not
  underlined. Character and visitor turns have parallel formatting; policy output
  is visibly separate. Keep labels as well as color where the distinction matters.
- Use the existing muted orange-red error accent. Keep feedback close to its action;
  a modal error should be visible within the modal and preserve correctable input.
- Keep footer key hints distinct from command explanations. Saving status belongs
  at the bottom right. Avoid product chrome that merely advertises implementation.
- Align grids with the adjacent pane. Two outputs fill the height; an odd final tile
  uses spare width. The active tile needs a clear border/title treatment. Metadata
  and navigation hints should not displace the grid or obscure content.
- Measure terminal cells with the existing ANSI/display-width helpers, not bytes
  or rune counts. Backend text offsets are Unicode code points; wrapping determines
  the visible row. Sanitize model text before terminal rendering.
- Preserve the 60×18 minimum-size gate and collapsed layout for narrow terminals.
  Focused fields, selected rows, bottom lines and controls must remain reachable.
  Don't resize or scroll a pane unexpectedly on every streamed token/state refresh.
- Show queued/running/completed/error states truthfully. Keep controls responsive
  while generation runs. UI-only refreshes must not start inference or mutate data.

## Work and proof

1. Identify the exact journey and state transitions being changed. Inspect the
   existing handler, rendering and nearest tests; don't redesign unrelated panes.
2. Make the smallest coherent change. Add focused interaction regression coverage
   when behavior changes, especially focus, Escape, selection and async event order.
3. Format Go changes and run focused Go tests. For Python/protocol changes, exercise
   both sides. Before a PR, run `make test lint build`.
4. For user-visible changes, run the relevant real terminal journey with disposable
   data (`CARLA_DATA_DIR` pointing to a temporary directory). This is temporary test
   isolation, not a maintained test workspace. Never instantiate a backend Project
   against someone's active workspace just to inspect it; initialization saves data.
5. Check the affected layout at narrow/wide sizes, resize, and exit. Test long text,
   Unicode, paste and path slashes when relevant. Streaming tests can use fake
   inference; don't launch expensive model jobs to verify layout alone.
6. Inspect golden-frame diffs before accepting them. Record what was actually
   exercised and any unverified behavior. Passing a snapshot is not proof that
   keyboard navigation or persistence works.

## Upstream

Adapted and substantially condensed for Carla from
[pageton/tui-design-skill](https://github.com/pageton/tui-design-skill/tree/ec54092ea3a47fb3aafe3abc3363bc5e911d1a14),
revision `ec54092ea3a47fb3aafe3abc3363bc5e911d1a14` (MIT; see [LICENSE](LICENSE)).
Carla owns this adaptation. Generic framework catalogs, starter applications,
provider configuration and unrelated examples are intentionally not vendored.
