package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

// Help documents commands independently of availability (busy/edit states).
var commandDescriptions = map[string]string{
	"behaviors":   "Create reusable behavior specs. Import a copy into a policy; detection rules stay with its behaviors and responses with the policy.",
	"policy":      "Configure Monitoring, Selection and Evals policies. Each policy has one judge and behavior specs. Monitoring checks during generation; Selection chooses alternatives; Evals assesses saved content.",
	"eval":        "Run /eval [policy] on selected document versions or checked conversations. In Evaluate, open New run setup with the focused dataset or selected items. Uses the active policy when no name is given. --train-on-pass true marks passing results for training; the policy default applies when omitted.",
	"evaluations": "Organize datasets in Data, open Evals policies in Policies, inspect Runs, and configure an evaluation with + New run. /export saves selected items or the opened collection with metadata.",
	"import":      "Add a local UTF-8 .txt or .md document to the shared Library. Title defaults to filename; author and source URL are optional.",
	"visitor":     "Write a visitor message in a new conversation fork. Open a conversation first.",
	"grid":        "Show concurrent Loom outputs. Arrows select a tile; Enter opens it. /grid returns.",
	"find":        "Filter documents or runs on the current page; type in the focused filter.",
	"active":      "Jump to the active generation while leaving background work running.",
	"rename":      "Name a document, conversation or Loom; clear the name to restore its automatic ancestry label.",
	"configure":   "Configure generation models, prompts, temperature, top-p and sampling; in Evaluate, configure the opened dataset. /policy owns monitoring, selection and Evals judges and behaviors. Loom flags override generation settings for one run.",
	"branch":      "Copy selected documents, conversations or sets without generation, preserving ancestry. Anthology opens the copies in Branches. Unavailable in Library and Evaluate. Save edits first.",
	"continue":    "Continue selected documents or conversations. Documents save a child revision; conversations advance in place with prior evidence retained. Anthology creates a new continuation in Branches without keeping it. Unavailable in Library and Evaluate. --loops repeats continuation; --tokens caps each completion. Simulator accepts --turns and --visitor. --model overrides this operation; --eval judges outputs. --monitoring on|off controls monitoring for this run; --selection off suppresses selection. Aliases: /generate, /run. Document judging is off unless --monitoring on or --eval is supplied; Simulator uses saved policy switches.",
	"loom":        "With a current target, /loom or /loom 1 continues it. A count of two or more forks the selected structure into alternatives. A set of four conversations with /loom 2 produces two sets of four. Anthology starts fresh Simulator conversations from selected kept documents. --loops repeats continuation on outputs; an enabled selection policy can choose whole alternatives between split loops. Library and Evaluate do not generate. --selection on|off and --monitoring on|off override policies for this run; selection needs two or more alternatives and explicit --loops N. Document judging is off unless --monitoring on, --selection on or --eval is supplied; Simulator uses saved policy switches.",
	"add":         "Library: import a source file. Branches: add selected versions to Anthology. Anthology: choose existing versions. Evaluate: add saved traces without judging.",
	"remove":      "Library: deselect sources. Anthology: unkeep versions. Branches: confirm deletion of versions and descendants. Evaluate: remove selected items, or confirm removal of a focused Data collection; sources, policies and Runs remain. Alias: /delete.",
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

// Help groups the action space, while search spans the complete command guide.
// The rows remain documentation: opening an example never executes it.
var helpGroups = []struct {
	id, title, summary string
	commands           []string
}{
	{"collections", "Collect & organize", "Add, remove, branch, rename and export saved material.", []string{"add", "remove", "branch", "rename", "snapshot", "clear"}},
	{"judging", "Policies & evaluation", "Configure a policy, assess a trace, then inspect Runs.", []string{"policy", "behaviors", "eval", "evaluations"}},
	{"settings", "Settings & session", "Saved defaults, models, workspaces, stopping and exiting.", []string{"configure", "models", "workspaces", "cancel", "restart", "exit", "keys"}},
	{"navigation", "Navigate & edit", "Focus versus selection, stage shortcuts and editing controls.", []string{"navigation", "library", "branches", "kept", "simulator", "evaluations", "grid", "active", "edit", "save", "discard", "inspect", "notes", "review", "find"}},
}

func (m *model) helpCommandRow(id string) row {
	text := m.helpText(id)
	purpose := strings.Split(commandDescriptions[id], ".")[0]
	label := "/" + commandName(action{id: id}) + " · " + purpose
	if id == "navigation" {
		label = "Focus, selection & keyboard navigation"
	}
	return row{id: id, label: label, preview: text}
}
func (m *model) helpHomeRows() []row {
	rows := []row{
		{id: "loom", label: "/loom · alternatives, flags & examples", preview: "Continue or split selected items.\n--tokens · --turns · --loops · --selection · --eval"},
		{id: "continue", label: "/continue · advance selected items", preview: "Advance the existing item or set.\n--tokens · --turns · --visitor · --monitoring"},
	}
	for _, group := range helpGroups {
		rows = append(rows, row{id: "group:" + group.id, label: group.title, preview: group.summary})
	}
	rows = append(rows, row{id: "group:all", label: "All commands & aliases", preview: "Browse every command, or type a command, alias or flag to search from any help list."})
	return rows
}
func (m *model) helpAllRows() []row {
	ids := []string{"loom", "continue", "navigation"}
	for _, a := range allActions {
		ids = append(ids, m.canonicalCommand(a.id))
	}
	ids = append(ids, "help", "keys", "cancel", "save", "discard", "exit", "restart")
	seen := map[string]bool{}
	var rows []row
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		rows = append(rows, m.helpCommandRow(id))
	}
	return rows
}
func (m *model) openHelp() tea.Cmd {
	m.dialog = &dialog{kind: "help", title: "Help · type a command or flag to search", rows: m.helpHomeRows()}
	return nil
}
func (m *model) openHelpRow() tea.Cmd {
	d := m.dialog
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	if strings.HasPrefix(r.id, "group:") {
		child := &dialog{kind: "help", title: r.label, parent: d}
		if r.id == "group:all" {
			child.rows = m.helpAllRows()
		} else {
			for _, g := range helpGroups {
				if r.id == "group:"+g.id {
					for _, id := range g.commands {
						child.rows = append(child.rows, m.helpCommandRow(id))
					}
				}
			}
		}
		m.dialog = child
	} else {
		title := "/" + commandName(action{id: r.id})
		if r.id == "navigation" {
			title = "Focus & selection"
		}
		m.dialog = &dialog{kind: "help-detail", title: title, parent: d, args: map[string]any{"text": m.helpText(r.id)}}
		// A flag search opens at its explanation, not at the top of a long guide.
		if strings.HasPrefix(d.query, "--") {
			for i, text := range m.helpLines() {
				if strings.Contains(strings.ToLower(ansi.Strip(text)), strings.ToLower(d.query)) {
					m.dialog.index = min(i, max(0, len(m.helpLines())-(m.dialogRect().h-5)))
					break
				}
			}
		}
	}
	return nil
}
func (m *model) filterHelp(msg tea.KeyPressMsg) bool {
	if msg.Code != tea.KeyBackspace && (msg.Text == "" || msg.Mod != 0) {
		return false
	}
	d := m.dialog
	if d.allRows == nil {
		d.allRows = append([]row{}, d.rows...)
	}
	if msg.Code == tea.KeyBackspace {
		r := []rune(d.query)
		if len(r) > 0 {
			d.query = string(r[:len(r)-1])
		}
	} else {
		d.query += msg.Text
	}
	if d.query == "" {
		d.rows = append([]row{}, d.allRows...)
	} else {
		d.rows = filterRows(m.helpAllRows(), d.query)
	}
	d.index = 0
	return true
}

func (m *model) helpText(id string) string {
	if id == "navigation" {
		return fmt.Sprintf("FOCUS & SELECTION\nArrows browse and preview. %s checks an item; checked items define command scope. Selecting a set includes its members.\n\n%s opens a document or selection actions. %s unwinds one level. %s / %s move between panes.\n\nSIMULATOR\nSelect a Loom parent to act on all its conversations. Select children for a subset. /clear removes the target so the next /loom starts fresh.\n\nEDITING\n%s saves; %s cancels the draft. /config keys changes bindings.\n\nGeneration actions are available in Branches, Anthology and Simulator; Library and Evaluate do not generate.", m.keyLabel("select"), m.keyLabel("nav.enter"), m.keyLabel("nav.back"), m.keyLabel("nav.next"), m.keyLabel("nav.prev"), m.keyLabel("save"), m.keyLabel("nav.back"))
	}
	var text string
	if id == "loom" || id == "continue" {
		text = m.generationHelp(id)
	} else {
		text = commandDescriptions[id]
		if args := m.commandArguments(id); len(args) > 0 {
			text += "\n\nSYNTAX\n/" + commandName(action{id: id}) + " " + strings.Join(args, " ")
		}
		if id == "eval" {
			text += "\n\nEXAMPLES\n/eval\n/eval \"Voice\" --train-on-pass true\n\nTARGET\nBranches/Simulator: saved selected items. Evaluate: opens New run setup. No implicit default dataset.\n\n--train-on-pass true|false\nOverride the policy's training-mark default for this run. Every enabled behavior must complete and pass."
		}
		if id == "configure" {
			text += "\n\nDEFAULTS\nBranches: Tokens, model, sampling and context. Simulator: Turns, Character tokens, Visitor tokens, speaker models and prompts. Flags override one run; /config saves defaults.\n\n/config keys\n/config model\n/config workspace"
		}
	}
	names := m.commandAliases(id)
	if len(names) > 1 {
		text += "\n\nALIASES\n/" + strings.Join(names[1:], " · /")
	}
	return text
}
func (m *model) generationHelp(id string) string {
	text := "Advance selected items. Documents save a revision; conversations advance their saved histories.\n\n/continue [flags]"
	if id == "loom" {
		text = "Continue once, or split into alternative futures.\n\n/loom [N] [flags]\nWith a target: N = 1 continues; N = 2+ splits.\nNo Simulator target: start N fresh conversations."
	}
	text += "\n\nGENERATION FLAGS\n--tokens N|Max\n  Cap new tokens per completion; not context size.\n--loops N\n  Repeat the operation. Omitted: one pass.\n--model alias\n  Override the document or character model.\n\nSIMULATOR FLAGS\n--turns N\n  Additional character replies per loop.\n--visitor \"text\"\n  Supply the next visitor message once, to each target.\n--visitor-model alias\n  Override the visitor model.\n\nPOLICY FLAGS\n--monitoring on|off\n  Enable or bypass the configured monitor this run.\n--eval \"policy name\"\n  Assess completed outputs with a named Evals policy.\n"
	if id == "loom" {
		text += "--selection on|off\n  Choose among alternatives. On requires N >= 2 and an explicit --loops N. --loops 1 selects once.\n"
	} else {
		text += "--selection off\n  Bypass selection. Continue cannot enable it.\n"
	}
	text += "\nDEFAULTS & SCOPE\n/config saves tokens, turns and model defaults. Flags affect only this run. Documents default to judging Off; Simulator inherits saved policy switches. --tokens controls output; context includes input plus output.\n\nBranches: continue or split selected documents. Anthology: /continue opens a new Branches continuation; /loom starts Simulator from kept documents. New versions are not automatically kept.\n\nSimulator: select a parent for the whole set, or children for a subset. Arrows only preview. /clear makes the next Loom fresh. /continue requires a target. Library and Evaluate do not generate.\n\nEXAMPLES\n/continue --tokens 512\n/loom 4 --tokens 512\n/loom 2 --tokens 512 --turns 3 --loops 2\n/continue --visitor \"Why?\" --turns 1\n/loom 4 --selection on --loops 1\n/loom 2 --eval \"Voice\" --monitoring off\n\nThe --turns and --visitor examples require Simulator (or Anthology Loom). Here --turns 3 --loops 2 adds six character replies per conversation. With selection Off, loops advance every output; with it On, later loops split again from the winning alternative.\n\nSPELLINGS\nFlags accept --flag=value and any order; examples put --loops last. --msg and --message mean --visitor."
	if id == "loom" {
		text += " --count N and -n N mean the positional count.\n\nSET EXAMPLE\nFour selected conversations + /loom 2 creates two alternative sets of four. /continue advances the original four. One output continues; two or more split alternatives."
	}
	return text
}

// Full command help is a read-only child view; Enter never executes a command.
func (m *model) helpLines() []string {
	var lines []string
	for _, paragraph := range strings.Split(m.dialog.args["text"].(string), "\n") {
		highlighted := strings.HasPrefix(paragraph, "--") || strings.HasPrefix(paragraph, "/")
		heading := paragraph != "" && paragraph == strings.ToUpper(paragraph) && !strings.HasPrefix(paragraph, " ")
		for _, wrapped := range strings.Split(ansi.Wrap(paragraph, m.dialogRect().w-4, ""), "\n") {
			if highlighted {
				wrapped = m.accent("#635A94", "#B6AADF").Render(wrapped)
			} else if heading {
				wrapped = bold.Render(wrapped)
			}
			lines = append(lines, wrapped)
		}
	}
	return lines
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
