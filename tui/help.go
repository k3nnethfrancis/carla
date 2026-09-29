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
	"continue":    "Advance the selected document or conversation set, retaining logical identity and preserving previous versions. Outside Simulator, Library selections compose one seed document; checked document versions advance independently. In Simulator, explicit selection wins over preview: clear selection to start fresh. --tokens caps each model completion; --turns counts new Character replies. --visitor supplies the next visitor message to each selected conversation; an unanswered message is a conflict. --model and --visitor-model override defaults for this operation; --eval judges the resulting versions. Aliases: /generate, /run.",
	"loom":        "Create N alternative futures of the selected item or set. A selected set of four conversations with /loom 3 creates three sets of four conversations. Fresh Simulator runs create N conversations. Document alternatives retain their source grouping. --tokens caps each completion; --turns counts new Character replies per loop; --loops repeats generation and selection. --visitor supplies the same next visitor message to each target. Model and evaluation flags apply to this operation only. Originals remain preserved.",
	"add":         "Library: import a source file. Branches: add selected versions to Anthology. Anthology: choose existing versions. Evaluate: add saved traces without judging.",
	"remove":      "Library: deselect sources. Anthology: unkeep versions. Branches: confirm deletion of versions and descendants. Evaluate: remove collection membership, retaining saved judgment history. Alias: /delete.",
	"keep":        "Keep checked branches (or the highlighted branch) in Anthology. In Evaluate, mark selected items for training.",
	"models":      "Choose the Loom base model; on Simulator choose Character or Visitor. /model visitor jumps directly to that picker.",
	"workspaces":  "Open a saved workspace or create a new one.",
	"edit":        "Edit existing document text or a conversation message; saving preserves the original as a new version.",
	"inspect":     "Show the exact generation inputs and provenance.",
	"review":      "Attach a verdict and note to this document or a text range.",
	"snapshot":    "Export selected saved items, or the current collection when nothing is selected. Saves content, provenance and judgment metadata to workspace files. Does not judge or train.",
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
