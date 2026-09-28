package main

import tea "charm.land/bubbletea/v2"

// Help documents commands independently of availability (busy/edit states).
var commandDescriptions = map[string]string{
	"policy":             "Configure monitoring, selection and saved whole-item evaluations. Jev behavior specs or local LLM judging prompts stay separate from generation settings.",
	"eval":               "Evaluate selected document versions, a conversation, or all conversations in a selected Loom. Choose a saved evaluation. --train-on-pass true marks passing results for training; default false.",
	"evaluations":        "Browse evaluated documents and conversations, filter results, add notes and mark items for training. /snapshot exports marked items and metadata.",
	"import":             "Add a local UTF-8 .txt or .md document to the shared Library. Title defaults to filename; author and source URL are optional.",
	"visitor":            "Write a visitor message in a new conversation fork. Open a conversation first.",
	"loom-policy":        "Configure Jev conversation dimensions, warnings, explicit stop rules and probability cutoffs. Alias: /loom-control-policy.",
	"grid":               "Show concurrent Loom outputs. Arrows select a tile; Enter opens it. /grid returns.",
	"find":               "Filter documents or runs on the current page; type in the focused filter.",
	"active":             "Jump to the active generation while leaving background work running.",
	"rename":             "Give this document a recognizable title without changing its text.",
	"character-sampling": "Character temperature, top-p, and output token budget.",
	"visitor-sampling":   "Visitor temperature, top-p, and output token budget.",
	"run":                "Simulator only: run conversations with the saved configuration (alias for /simulate).",
	"configure":          "Temperature, top-p and shared document settings in Library, Branches and Anthology; conversation settings in Simulator. Selection and monitoring are separate policies. Command flags override settings for one run.",
	"generate":           "Alias for /continue; uses the current saved document and cursor.",
	"continue":           "Continue from cursor. /continue 512 sets the maximum output tokens.",
	"branch":             "Fork the selected conversation in Simulator, or saved document in Branches, without generation. Save edits first.",
	"loom":               "Generate one continuation or Character reply. /loom 3 --tokens 512 --loops 4 creates three alternatives per loop. Simulator also accepts --turns N. A batch requires choosing a conversation.",
	"add":                "Add highlighted source passages to the workspace seed set.",
	"remove":             "Library: deselect sources. Anthology: unkeep versions. Branches: confirm deletion of versions and descendants. Alias: /delete.",
	"delete":             "Review checked branches and descendants, then confirm deletion. Saves a recovery snapshot.",
	"keep":               "Keep checked branches in the anthology, otherwise the highlighted branch.",
	"grow":               "Run bounded continuation rounds with the local selection policy.",
	"settings":           "Loom branch count, output length, sampling and context; on Simulator opens its configuration.",
	"models":             "Choose the Loom base model; on Simulator choose Character or Visitor. /model visitor jumps directly to that picker.",
	"workspaces":         "Open a saved workspace or create a new one.",
	"edit":               "Edit existing document text or a conversation message; saving preserves the original as a new version.",
	"inspect":            "Show the exact generation inputs and provenance.",
	"review":             "Attach a verdict and note to this document or a text range.",
	"spec":               "Edit the criteria used by the continuation selection policy.",
	"prompt":             "Edit the local selector model’s prompt.",
	"snapshot":           "Export anthology documents with their provenance.",
	"clear":              "Clear selected seed passages; saved branches remain.",
	"library":            "Browse seed documents and select passages.",
	"branches":           "Browse continuations; LEFT collapses, RIGHT expands, ENTER opens.",
	"kept":               "Browse the anthology of kept document versions.",
	"notes":              "Open this document’s notes. + adds a note; Enter reads and jumps to its anchor.",
	"simulator":          "Configure and inspect raw-document conversation runs.",
	"sim-config":         "Select anthology versions, models, opening, prompts and per-speaker sampling.",
	"simulate":           "Run the saved Simulator configuration.",
	"grow-config":        "Configure the branch generator, selector, criteria, cycles and sampling.",
	"grow-policy":        "Edit Grow selection criteria; never injected into the base generator.",
	"help":               "Show all commands and keyboard navigation.",
	"keys":               "Assign keys to actions and navigation; bindings apply across workspaces.",
	"cancel":             "Stop the active generation; preserve its partial output.",
	"save":               "Save the active document or policy edit (editing only).",
	"discard":            "Cancel the draft and return to document navigation; also ESC.",
	"restart":            "Restart Carla in this workspace; stop active generation and preserve partials.",
	"exit":               "Exit Carla; preserves unsaved drafts in recovery files.",
	"quit":               "Close Carla; preserves unsaved drafts in recovery files.",
}

func (m *model) openHelp() tea.Cmd {
	d := &dialog{kind: "help", title: "Commands · ↑↓ browse · ESC close"}
	d.rows = append(d.rows, row{label: "Navigation", preview: "TAB / SHIFT+TAB: panels · ESC: sections · ←→: switch · ENTER/↓: enter"})
	actions := append([]action{}, allActions...)
	for _, id := range []string{"help", "keys", "cancel", "save", "discard", "quit", "exit", "restart"} {
		actions = append(actions, action{id: id})
	}
	seen := map[string]bool{}
	for _, a := range actions {
		a.id = m.canonicalCommand(a.id)
		if seen[a.id] {
			continue
		}
		seen[a.id] = true
		d.rows = append(d.rows, row{id: a.id, label: "/" + commandName(a), preview: commandDescriptions[a.id]})
	}
	m.dialog = d
	return nil
}
