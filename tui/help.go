package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

// Help documents commands independently of availability (busy/edit states).
var commandDescriptions = map[string]string{
	"policy":      "Configure monitoring, selection and reusable judge configurations. Jev behavior specs or local LLM judging prompts stay separate from generation settings.",
	"eval":        "Run /eval [name] on selected document versions or checked conversations. Uses the active evaluation when no name is given. --train-on-pass true marks passing results for training; default false.",
	"evaluations": "Browse named evaluation collections, add existing material without judging, run selected/pending items, review evidence and mark items for training. /snapshot exports marked items and metadata.",
	"import":      "Add a local UTF-8 .txt or .md document to the shared Library. Title defaults to filename; author and source URL are optional.",
	"visitor":     "Write a visitor message in a new conversation fork. Open a conversation first.",
	"grid":        "Show concurrent Loom outputs. Arrows select a tile; Enter opens it. /grid returns.",
	"find":        "Filter documents or runs on the current page; type in the focused filter.",
	"active":      "Jump to the active generation while leaving background work running.",
	"rename":      "Give this document a recognizable title without changing its text.",
	"configure":   "Configure generation models, prompts, temperature, top-p and sampling; in Evaluate, configure the opened collection and its judges. /policy owns monitoring and selection. Loom flags override generation settings for one run.",
	"branch":      "Fork the selected conversation in Simulator, or saved document in Branches, without generation. Save edits first.",
	"loom":        "Generate alternatives from one starting point: document continuations outside Simulator, conversation extensions inside it. A checked Loom or subset continues each conversation independently. Defaults: one fresh alternative, one loop; selected conversations use configured turns. The new output group stays selected. --tokens caps each generation; --turns counts Character replies per loop; --loops repeats generation and policy selection. --eval judges completed outputs with a named evaluation. --msg/--message supplies a fresh Visitor opener (clear selection first). Quote names/messages containing spaces. Aliases /continue, /generate, /run, /simulate and /grow use the same syntax.",
	"add":         "Add highlighted source passages to the workspace seed set.",
	"remove":      "Library: deselect sources. Anthology: unkeep versions. Branches: confirm deletion of versions and descendants. Evaluate: unmark training items. Alias: /delete.",
	"keep":        "Keep checked branches (or the highlighted branch) in Anthology. In Evaluate, mark selected items for training.",
	"models":      "Choose the Loom base model; on Simulator choose Character or Visitor. /model visitor jumps directly to that picker.",
	"workspaces":  "Open a saved workspace or create a new one.",
	"edit":        "Edit existing document text or a conversation message; saving preserves the original as a new version.",
	"inspect":     "Show the exact generation inputs and provenance.",
	"review":      "Attach a verdict and note to this document or a text range.",
	"snapshot":    "Export anthology documents, or training-marked items from the opened evaluation with judgment history and metadata. Does not train a model.",
	"clear":       "Clear source/branch selection. In Simulator clear the conversation target so the next Loom starts fresh. Does not delete saved content.",
	"library":     "Browse seed documents and select passages.",
	"branches":    "Browse continuations; LEFT collapses, RIGHT expands, ENTER opens.",
	"kept":        "Browse the anthology of kept document versions.",
	"notes":       "Open this document’s notes. + adds a note; Enter reads and jumps to its anchor.",
	"simulator":   "Configure and inspect raw-document conversation runs.",
	"help":        "Show all commands and keyboard navigation.",
	"keys":        "Assign keys to actions and navigation; bindings apply across workspaces.",
	"cancel":      "Stop the active generation; preserve its partial output.",
	"save":        "Save the active document or policy edit (editing only).",
	"discard":     "Cancel the draft and return to document navigation; also ESC.",
	"restart":     "Restart Carla in this workspace; stop active generation and preserve partials.",
	"exit":        "Exit Carla; preserves unsaved drafts in recovery files.",
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
		d.rows = append(d.rows, row{id: a.id, label: "/" + commandName(a), preview: m.commandHelp(a.id)})
	}
	m.dialog = d
	return nil
}

// Full command help is a read-only child view; Enter never executes a command.
func (m *model) helpLines() []string {
	return strings.Split(ansi.Wrap(m.dialog.args["text"].(string), m.dialogRect().w-4, ""), "\n")
}
func (m *model) helpKey(msg tea.KeyPressMsg) tea.Cmd {
	limit := max(0, len(m.helpLines())-(m.dialogRect().h-5))
	switch m.navigationKey(msg.String()) {
	case "nav.back":
		m.closeDialog()
		return nil
	case "nav.up":
		m.dialog.index--
	case "nav.down":
		m.dialog.index++
	default:
		switch msg.String() {
		case "pgdown":
			m.dialog.index += m.dialogRect().h - 5
		case "pgup":
			m.dialog.index -= m.dialogRect().h - 5
		case "home":
			m.dialog.index = 0
		case "end":
			m.dialog.index = limit
		}
	}
	m.dialog.index = max(0, min(m.dialog.index, limit))
	return nil
}

func (m *model) scrollHelp(delta int) {
	limit := max(0, len(m.helpLines())-(m.dialogRect().h-5))
	m.dialog.index = max(0, min(m.dialog.index+delta, limit))
}
