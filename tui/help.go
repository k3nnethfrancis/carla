package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

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
