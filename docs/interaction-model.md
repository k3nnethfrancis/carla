# Interaction model

Commands operate on explicit targets: one document or conversation, a set, or a
nested set. Containment defines operation scope; ancestry records where content
came from. The executor enumerates leaves to schedule work while preserving the
selected tree in saved provenance.

## Where actions work

| Stage | Branch | Continue | Loom |
| --- | --- | --- | --- |
| Library | — | — | — |
| Branches | Copy with ancestry, no generation | Save a continuation beneath the document | Continue once; 2+ creates alternatives |
| Anthology | Copy into Branches | Continue into a new branch, not automatically kept | Start fresh Simulator conversations from selected documents |
| Simulator | Fork selected conversation structure without generation | Advance selected conversations | Continue selected conversations; 2+ forks their structure |
| Evaluate | — | — | — |

In Library, Space selects source passages and Enter opens them together in
Branches. Opening the same selection again creates another seed root with the
same source provenance. Library content remains unchanged.

## Scope and selection

Arrows preview; Space selects. Checked targets take precedence over preview.
Opening a conversation targets that conversation. Clearing Simulator selection
returns to fresh generation. Moving to the command bar preserves the scope.

A document checkbox selects that exact version, not its ancestry descendants.
A set checkbox selects its contained members, preserving nested boundaries.
Deletion separately previews affected descendants before confirmation.

Set identity does not change when the highlighted row moves. Current document
sets resolve their members' current logical heads; historical membership snapshots
remain frozen. Anthology and evaluation entries reference exact saved versions.

## Advance and split

For a selected set A of four conversations:

```text
/continue   A → A: each selected conversation advances
/loom       A → A: same operation
/loom 1     A → A: same operation
/loom 2     A → a new Loom containing two alternative sets of four
/branch     A → a copied structure, without generation
```

Selecting one conversation narrows the same operations to that conversation.
Selecting sets of sets preserves those internal boundaries in each alternative.
Conversation state lives at the leaves. Grouping does not concatenate conversations
or combine their histories into one model request. Loom groups use stable numbered
labels with nested alternatives. Opening a group shows its conversations in a
paged grid; opening a tile narrows the action target to that conversation, and
Escape returns to the same group grid.

Documents retain immutable child versions for continuations. Sources and old
versions keep their existing futures. An automatic scroll to the first change
is only a reading aid; deliberately positioning the reader cursor selects a
prefix for generation. Anthology Branch/Continue starts independent material in
Branches which must be kept separately.

## Repetition and policies

```text
/continue --tokens 512 --loops 3
/loom 4 --tokens 512 --loops 3
/loom 2 --visitor "What matters to you?" --turns 2 --loops 3
```

Without selection, a split creates N alternatives once, then later loops advance
those outputs. It does not multiply N on every loop. With Selection enabled in
`/policy`, split loops judge complete alternatives, select one, then split again
from that winner. One-output loops always continue directly. Selection defaults
to Off and is separate from optional monitoring.

`--tokens` caps each model completion. `--turns` counts additional character replies
per Simulator loop; it is unavailable for document continuations. `--visitor`
supplies the next visitor message once per selected conversation. An unanswered
visitor message is a conflict, not silently replaced. Model flags override only
this operation. `--eval` judges generated versions without changing prompts.

## Collection actions and evidence

`/add` imports sources or adds exact versions to the current collection.
`/remove` removes membership or, in Branches, confirms deletion of documents and
affected descendants. Delete is its default panel binding. In text editors Delete
edits text; saved custom bindings remain respected. `/eval` judges individual saved
items; selection policies can instead compare complete alternative structures.
`/export` freezes selected items or the collection into a provenance-bearing bundle.

Continue retains prior text, messages, model settings and evidence. New content
does not inherit a passing judgment, training mark or Anthology membership.
Exports are not a training algorithm or a supported workspace restore format.

## Ownership

Go resolves focus and explicit selection into a structured scope. Python validates
that scope, freezes generation inputs, owns revisions and group provenance, and
schedules leaves with bounded concurrency. The same command handler serves
keyboard actions and the command palette. Stopping retains saved partial work.

### Document browsing and editing

The preview label identifies the **focused** item. Space checks items into an
explicit selection; the pane shows the selected count. Enter with checked
branches opens their actions, including deletion with confirmation. Single mouse
clicks focus list rows; Enter opens or activates them. Document notes offer New
note and Edit document; deletion belongs to the selected-item actions.

While editing, Backspace and Delete edit text. Save with Ctrl+Enter, Ctrl+S, or
`/save`; Command+Enter also saves when the terminal reports its modifier (Super).
Some terminals intercept Command+Enter or send plain Enter, which remains a
newline. Escape cancels the draft. Shortcuts can be changed in keybindings.
