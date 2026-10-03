package main

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func (m *model) noteRows() []row {
	rows := []row{{id: "back", kind: "note-back", label: "← Branches"}, {id: "new", kind: "note-new", label: "+ New note"},
		{id: "edit-document", kind: "document-edit", label: "Edit document", preview: "Edit this document. Save commits a new version; Escape cancels."},
		{id: "rename-document", kind: "document-rename", label: "Rename document", preview: "Choose a memorable name. Clear it to restore the automatic ancestry name."}}
	for _, a := range m.data.Annotations {
		if a.Node != m.currentID() {
			continue
		}
		label := strings.Join(strings.Fields(a.Note), " ")
		if label == "" {
			label = a.Verdict
		}
		rows = append(rows, row{id: a.ID, kind: "note", label: label, preview: a.Note})
	}
	return rows
}
func (m *model) openNotes(focus bool) tea.Cmd {
	if m.data.Current == nil || (m.section != 1 && m.section != 2) {
		m.status = "Open a document to see its notes"
		return nil
	}
	m.notesOpen = true
	m.selected = 0
	m.noteScroll = 0
	m.showInspector = false
	if focus {
		m.focus = 0
		m.editor.Blur()
	}
	m.reflow()
	return nil
}
func (m *model) backFromNotes() tea.Cmd {
	if m.pending {
		m.status = "Wait for the current operation before returning"
		return nil
	}

	if m.editing == "document" {
		m.status = "Use /save or /cancel before leaving this edit"
		return nil
	}
	m.notesOpen = false
	m.section, m.focus = 1, 0
	m.branchSelection = map[string]bool{}
	m.selectDocument(m.currentID())
	m.reflow()
	return nil
}
func (m *model) activateNote() tea.Cmd {
	rows := m.noteRows()
	if len(rows) == 0 {
		return nil
	}
	r := rows[min(m.selected, len(rows)-1)]
	switch r.kind {
	case "note-back":
		return m.backFromNotes()
	case "document-rename":
		return m.renameDocument()
	case "document-edit":
		return m.editDocumentWithNotes()
	case "note-new":
		if m.editing == "document" {
			m.status = "Use /save before attaching a note to these edits"
			return nil
		}
		if m.pending || m.data.Busy {
			return nil
		}
		offset := m.cursorOffset()
		args := map[string]any{"node": m.currentID()}
		if m.editing == "document" {
			args["text"] = m.editor.Value()
			offset = textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column())
		}
		args["offset"] = offset
		d := &dialog{kind: "note-new", title: "New note", args: args}
		d.add("Note", "")
		m.dialog = d
		return d.fields[0].input.Focus()
	case "note":
		if m.pending {
			return nil
		}
		for _, a := range m.data.Annotations {
			if a.ID != r.id {
				continue
			}
			m.revealNote(a)
			if m.data.Busy {
				m.status = "Read-only while generation runs"
				return nil
			}
			d := &dialog{kind: "note-edit", title: "Edit note", args: map[string]any{"id": a.ID, "node": a.Node}}
			d.add("Note", a.Note)
			m.dialog = d
			return d.fields[0].input.Focus()
		}

	}
	return nil
}
func moveTextCursor(area *textarea.Model, offset int) {
	text := []rune(area.Value())
	offset = max(0, min(offset, len(text)))
	prefix := string(text[:offset])
	target := strings.Count(prefix, "\n")
	area.MoveToBegin()
	for area.Line() < target {
		previous := area.Line()
		// Skip wrapped display rows when seeking a logical text position.
		area.CursorEnd()
		area.CursorDown()
		if area.Line() == previous {
			break
		}
	}
	col := len([]rune(prefix))
	if at := strings.LastIndex(prefix, "\n"); at >= 0 {
		col = len([]rune(prefix[at+1:]))
	}
	area.SetCursorColumn(col)
}
func (m *model) notesKey(msg tea.KeyPressMsg) tea.Cmd {
	key := m.navigationKey(msg.String())
	if m.saveKey(msg) && m.editing == "document" {
		return m.saveEditor()
	}
	switch key {
	case "nav.enter":
		return m.activateNote()
	case "nav.back":
		if m.editing != "" {
			return m.cancelEdit()
		}
		return m.backFromNotes()
	case "nav.next", "nav.prev":
		m.focus = 1
		m.reflow()
		if m.editing == "document" {
			return m.editor.Focus()
		}
		return nil
	case "nav.up", "nav.down":
		step := 1
		if key == "nav.up" {
			step = -1
		}
		m.selected = max(0, min(len(m.noteRows())-1, m.selected+step))
		m.noteScroll = 0
		return nil
	}
	if msg.String() == "+" {
		m.selected = 1
		return m.activateNote()
	}
	if msg.Code == tea.KeyPgDown {
		m.noteScroll += 5
	}
	if msg.Code == tea.KeyPgUp {
		m.noteScroll = max(0, m.noteScroll-5)
	}
	return nil
}
func (m *model) notesListHeight(r rect) int { return min(len(m.noteRows()), max(2, (r.h-6)/2)) }

func (m *model) notesStart(r rect) int {
	return max(2, m.selected-max(1, m.notesListHeight(r)-2)+1)
}
func (m *model) notesClick(y int, r rect) tea.Cmd {
	visible := m.notesListHeight(r)
	index := y
	if y >= 2 {
		index = m.notesStart(r) + y - 2
	}
	if y >= 0 && y < visible && index < len(m.noteRows()) {
		m.focus = 0
		m.selected = index
		m.noteScroll = 0
		m.editor.Blur()
		m.reflow()
		// Action rows are buttons; saved notes remain focus-first list items.
		switch m.noteRows()[index].kind {
		case "note-back", "note-new", "document-edit", "document-rename":
			return m.activateNote()
		}
		return nil
	}
	return nil
}
func (m *model) notesView(r rect) string {
	rows := m.noteRows()
	visible := m.notesListHeight(r)
	lines := []string{}
	indices := []int{0, 1}
	for i := m.notesStart(r); i < min(len(rows), m.notesStart(r)+visible-2); i++ {
		indices = append(indices, i)
	}
	for _, i := range indices {
		label := line(safe(rows[i].label), r.w-4)
		if i == m.selected {
			label = selectedStyle.Render(label)
		}
		lines = append(lines, label)
	}
	lines = append(lines, "")
	text := "Select a note to read it.\n+ adds a note at your cursor."
	if m.selected >= 2 && m.selected < len(rows) {
		text = rows[m.selected].preview
	}
	wrapped := strings.Split(ansi.Wrap(safe(text), r.w-4, ""), "\n")
	room := max(1, r.h-5-len(lines))
	offset := min(m.noteScroll, max(0, len(wrapped)-room))
	lines = append(lines, wrapped[offset:min(len(wrapped), offset+room)]...)
	return strings.Join(lines, "\n")
}

func (m *model) revealNote(a annotation) {
	m.noteScroll = 0
	if m.editing == "document" {
		moveTextCursor(&m.editor, a.Start)
	} else {
		moveTextCursor(&m.navigator, a.Start)
		m.focus = 1
		m.reflow()
		m.revealCursor()
		m.focus = 0
	}
	m.reflow()
}

func (m *model) renameDocument() tea.Cmd {
	if m.data.Current == nil || m.pending {
		return nil
	}
	d := &dialog{kind: "rename", title: "Rename document", args: map[string]any{"node": m.currentID()}}
	d.add("Name (blank restores automatic)", m.data.Current.Title)
	d.add("Update child ancestry names (SPACE toggles)", "Off")
	m.dialog = d
	return d.fields[0].input.Focus()
}
