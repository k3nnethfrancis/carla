package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m *model) openDialog(kind string) tea.Cmd {
	if m.editing != "" {
		m.status = "Use /save (CTRL+ENTER) or /cancel (ESC) first"
		return nil
	}
	if m.data.Busy {
		m.status = "Stop the active operation first"
		return nil
	}
	d := &dialog{kind: kind, args: map[string]any{}}
	switch kind {
	case "workspaces":
		d.title = "Workspaces"
		for _, w := range m.data.Workspaces {
			label := w.Name
			if w.Path == m.data.Workspace.Path {
				label += " · current"
			}
			d.rows = append(d.rows, row{id: w.Path, label: label, preview: w.Path})
		}
		d.rows = append(d.rows, row{id: "new", label: "+ New workspace"})
	case "models":
		d.title = "Base model"
		for _, model := range m.data.Models {
			label := model.Name
			if model.Alias == m.data.ModelAlias {
				label += " · current"
			}
			d.rows = append(d.rows, row{id: model.Alias, label: label, preview: "Loom generator · raw base completion. Grow selector: " + m.data.PolicyModel})
		}
		d.rows = append(d.rows, row{id: "setup", label: "+ Add model", preview: "Download a GGUF or use a local file."})
	case "new":
		d.title = "New workspace"
		d.add("Name", "")
	case "settings":
		d.title = "Generation settings"
		s := m.data.Settings
		d.add("Sibling branches", strconv.Itoa(s.Count))
		output := strconv.Itoa(s.Tokens)
		if s.Tokens == -1 {
			output = "Max"
		}
		d.add("Output tokens", output)
		d.add("Temperature", fmt.Sprint(s.Temperature))
		d.add("Top-p", fmt.Sprint(s.TopP))
		context := strconv.Itoa(m.data.ModelContext)
		if m.data.ModelContext == 0 {
			context = "Default"
		}
		label := "Context (default unavailable)"
		if m.data.NativeContext > 0 {
			label = fmt.Sprintf("Context (default: %s)", tokenNumber(m.data.NativeContext))
		}
		d.add(label, context)
	case "review":
		if m.data.Current == nil {
			return nil
		}
		d.title = "Review current document"
		d.add("Verdict (unreviewed / promising / pass)", "promising")
		d.add("Note", "")
		d.add("Start character", "0")
		d.add("End character", strconv.Itoa(len([]rune(m.currentText()))))
	}
	m.dialog = d
	if kind == "settings" {
		return nil
	}
	if len(d.fields) > 0 {
		return d.fields[0].input.Focus()
	}
	return nil
}
func (d *dialog) add(label, value string) {
	i := newInput()
	i.SetValue(value)
	i.CharLimit = 0
	i.Prompt = ""
	d.fields = append(d.fields, field{label, i})
}
func (m *model) submitDialog() tea.Cmd {
	if m.dialog != nil && strings.HasPrefix(m.dialog.kind, "setup-") {
		return m.submitSetup(m.dialog)
	}
	if m.dialog != nil && (m.dialog.kind == "models" || m.dialog.kind == "sim-model" || m.dialog.kind == "selector-pick") && len(m.dialog.rows) > 0 && m.dialog.rows[m.dialog.index].id == "setup" {
		m.setupReturn = m.dialog
		m.dialog = nil
		return m.send("setup.open", nil)
	}
	if m.dialog != nil && strings.HasPrefix(m.dialog.kind, "loom-policy") {
		return m.submitLoomPolicy(m.dialog)
	}
	d := m.dialog
	if d.kind == "config-number" {
		return m.saveNumber(d)
	}
	if d.kind == "sim-documents" {
		selected := d.args["selected"].(map[string]bool)
		ids := []string{}
		rows := d.rows
		if d.allRows != nil {
			rows = d.allRows
		}
		for _, r := range rows {
			if selected[r.id] {
				ids = append(ids, r.id)
			}
		}
		return m.saveDialog(d, "simulator.configure", map[string]any{"documents": ids})
	}
	if d.kind == "settings" && d.adjusting {
		m.finishSetting(false)
		return nil
	}
	if d.kind == "selection" {
		id := d.rows[d.index].id
		m.dialog = nil
		return m.selectionAction(id)
	}
	if d.kind == "delete" {
		switch d.rows[d.index].id {
		case "cancel":
			m.dialog = nil
		case "confirm":
			if m.pending || m.data.Busy {
				return nil
			}
			args := d.args
			m.dialog = nil
			return m.send("node.delete", args)
		}
		return nil
	}
	if d.kind == "help" {
		return nil
	}
	if len(d.fields) > 0 {
		values := []string{}
		for _, f := range d.fields {
			values = append(values, f.input.Value())
		}
		var command string
		args := d.args
		switch d.kind {
		case "rename":
			command = "node.rename"
			args["title"] = values[0]
		case "sim-text":
			command = "simulator.configure"
			args = map[string]any{d.args["field"].(string): values[0]}
		case "note-new", "note-edit":
			if m.pending || m.data.Busy {
				m.status = "Wait for the current operation"
				return nil
			}
			if strings.TrimSpace(values[0]) == "" {
				m.status = "Write a note first"
				return nil
			}
			args["note"] = values[0]
			if d.kind == "note-edit" {
				command = "note.update"
			} else {
				command = "note.add"
				m.notePending = true
			}
		case "new":
			command = "workspace.open"
			args = map[string]any{"name": values[0], "create": true}
		case "settings":
			nums := make([]float64, len(values))
			for i, v := range values {
				if i == 1 && strings.EqualFold(strings.TrimSpace(v), "Max") {
					nums[i] = -1
					continue
				}
				if i == 4 && strings.EqualFold(strings.TrimSpace(v), "Default") {
					nums[i] = 0
					continue
				}
				n, err := strconv.ParseFloat(v, 64)
				if err != nil {
					m.status = "Enter a number for " + d.fields[i].label
					return nil
				}
				nums[i] = n
			}
			for _, i := range []int{0, 1, 4} {
				if nums[i] != float64(int(nums[i])) {
					m.status = "Branch, token and round counts must be whole numbers"
					return nil
				}
			}
			command = "configure"
			args["model_context"] = int(nums[4])
			args["settings"] = settings{int(nums[0]), int(nums[1]), nums[2], nums[3], m.data.Settings.Rounds}
		case "review":
			start, e1 := strconv.Atoi(values[2])
			end, e2 := strconv.Atoi(values[3])
			if e1 != nil || e2 != nil {
				m.status = "Character offsets must be whole numbers"
				return nil
			}
			command = "annotate"
			args["verdict"], args["note"], args["start"], args["end"] = strings.TrimSpace(values[0]), values[1], start, end
			m.editing = ""
			m.editor.Blur()
		}
		return m.saveDialog(d, command, args)
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	m.dialog = nil
	switch d.kind {
	case "sim-openings", "sim-opening-mode", "sim-config", "sim-speakers", "grow-config", "sim-model", "selector-pick", "sim-sampling":
		return m.configureChoice(d, r)
	case "conversation-edit":
		index, _ := strconv.Atoi(r.id)
		m.conversationEdit = index
		return m.startConversationEdit(m.simulation.Conversations[m.gridSelection].Turns[index].Text)
	case "document-actions":
		if r.id == "edit" {
			return m.editDocumentWithNotes()
		}
		return m.openNotes(true)
	case "workspaces":
		if r.id == "new" {
			return m.openDialog("new")
		}
		return m.send("workspace.open", map[string]any{"path": r.id})
	case "models":
		return m.saveDialog(d, "configure", map[string]any{"model_alias": r.id})
	}
	return nil
}
func (m *model) dialogKey(msg tea.KeyPressMsg) tea.Cmd {
	d := m.dialog
	if d.kind == "config-number" {
		return m.numberKey(msg)
	}
	if d.kind == "sim-documents" && (msg.Code == tea.KeySpace || msg.Code == tea.KeyEnter) {
		if len(d.rows) == 0 {
			return nil
		}
		r := &d.rows[d.index]
		selected := d.args["selected"].(map[string]bool)
		selected[r.id] = !selected[r.id]
		r.label = strings.TrimPrefix(r.label, "✓ ")
		if selected[r.id] {
			r.label = "✓ " + r.label
		}
		for i, old := range d.allRows {
			if old.id == r.id {
				d.allRows[i] = *r
			}
		}
		return nil
	}
	if d.kind == "settings" {
		return m.settingsKey(msg)
	}
	key := m.navigationKey(msg.String())
	if msg.String() == "ctrl+s" || m.boundAction(msg.String(), "editor") == "save" {
		return m.submitDialog()
	}
	switch key {
	case "nav.back":
		return m.closeDialog()
	case "nav.enter":
		if len(d.fields) > 0 && d.field < len(d.fields)-1 {
			d.fields[d.field].input.Blur()
			d.field++
			return d.fields[d.field].input.Focus()
		}
		return m.submitDialog()
	case "nav.up", "nav.down", "nav.next", "nav.prev":
		step := 1
		if key == "nav.up" || key == "nav.prev" {
			step = -1
		}
		if len(d.fields) > 0 {
			d.fields[d.field].input.Blur()
			d.field = (d.field + step + len(d.fields)) % len(d.fields)
			return d.fields[d.field].input.Focus()
		}
		if len(d.rows) > 0 {
			d.index = (d.index + step + len(d.rows)) % len(d.rows)
		}
		return nil
	}
	if m.filterDialog(msg) {
		return nil
	}
	if len(d.fields) > 0 {
		var cmd tea.Cmd
		d.fields[d.field].input, cmd = d.fields[d.field].input.Update(msg)
		return cmd
	}
	return nil
}

// A review can attach to a selected passage in the editor without injecting that
// feedback into the base-model prompt. Offsets refer to the saved edited version.
func (m *model) reviewSelection() tea.Cmd {
	if m.editing != "document" {
		return nil
	}
	text := m.editor.Value()
	start, end := 0, len([]rune(text))
	if a, b, ok := m.editor.Selection(); ok {
		start = textOffset(text, a.Row, a.Col)
		end = textOffset(text, b.Row, b.Col)
	}
	d := &dialog{kind: "review", title: "Review selected passage", args: map[string]any{"node": m.editNode, "text": text}}
	d.add("Verdict (unreviewed / promising / pass)", "promising")
	d.add("Note", "")
	d.add("Start character", strconv.Itoa(start))
	d.add("End character", strconv.Itoa(end))
	m.dialog = d
	m.reflow()
	return d.fields[0].input.Focus()
}

func newInput() textinput.Model {
	i := textinput.New()
	i.SetVirtualCursor(true)
	plain := textinput.StyleState{Placeholder: dim}
	i.SetStyles(textinput.Styles{Focused: plain, Blurred: plain})
	return i
}

// Group token counts for scanning large context windows.
func tokenNumber(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s + " tokens"
}
