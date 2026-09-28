package main

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func (m *model) toggleTarget() tea.Cmd {
	if m.section == 3 {
		if target, ok := m.conversationTarget(); ok {
			chosen := conversationParent{Run: target["run"].(string), Conversation: target["conversation"].(int)}
			if m.loomConversation != nil && *m.loomConversation == chosen {
				m.loomConversation = nil
			} else {
				m.loomConversation = &chosen
			}
		}
		m.reflow()
		return nil
	}
	if m.section == 4 && !m.data.Busy {
		r := m.targetRow()
		if r.kind == "evaluation" {
			if m.evalSelection == nil {
				m.evalSelection = map[string]bool{}
			}
			m.evalSelection[r.id] = !m.evalSelection[r.id]
		}
		m.reflow()
		return nil
	}
	if m.notesOpen {
		return nil
	}
	if (m.section == 1 || m.section == 2) && !m.pending && !m.data.Busy {
		r := m.targetRow()
		if r.kind == "node" {
			ids := m.subtree([]string{r.id})
			all := true
			for id := range ids {
				if !m.branchSelection[id] {
					all = false
				}
			}
			for id := range ids {
				if all {
					delete(m.branchSelection, id)
				} else {
					m.branchSelection[id] = true
				}
			}
		}
		m.reflow()
		return nil
	}
	if m.section != 0 || m.pending || m.data.Busy {
		return nil
	}
	refs := m.targetRefs()
	if len(refs) == 0 {
		return nil
	}
	selected := map[string]bool{}
	for _, ref := range m.data.Selected {
		selected[ref] = true
	}
	all := true
	for _, ref := range refs {
		if !selected[ref] {
			all = false
		}
	}
	command := "seed.add"
	if all {
		command = "seed.remove"
	}
	return m.send(command, map[string]any{"refs": refs})
}
func (m *model) cursorActive() bool {
	return m.focus == 1 && (m.section == 1 || m.section == 2) && m.editing == "" && !m.data.Busy && m.data.Current != nil
}
func (m *model) cursorOffset() int {
	if m.editing == "document" {
		return textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column())
	}
	return textOffset(m.navigator.Value(), m.navigator.Line(), m.navigator.Column())
}
func (m *model) syncCursor(width, height int) {
	m.navigator.SetWidth(width)
	m.navigator.SetHeight(height)
	if m.editing == "" && m.data.Current != nil && (m.cursorNode != m.currentID() || m.navigator.Value() != m.currentText()) {
		m.cursorNode = m.currentID()
		m.navigator.SetValue(m.currentText())
		m.navigator.CursorEnd()
	}
}
func (m *model) moveCursor(key string, msg tea.KeyPressMsg) tea.Cmd {
	move := navigationMessage(key, msg)
	switch move.Code {
	case tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown:
	default:
		return nil // Document browsing never edits text, including paste.
	}
	var cmd tea.Cmd
	m.navigator, cmd = m.navigator.Update(move)
	m.reflow()
	m.revealCursor()
	return cmd
}
func (m *model) revealCursor() {
	if !m.cursorActive() {
		return
	}
	text := []rune(m.currentText())
	offset := min(len(text), m.cursorOffset())
	row := strings.Count(ansi.Wrap(safe(string(text[:offset]))+"█", max(1, m.document.Width()), ""), "\n")
	if row < m.document.YOffset() {
		m.document.SetYOffset(row)
	} else if row >= m.document.YOffset()+m.document.Height() {
		m.document.SetYOffset(row - m.document.Height() + 1)
	}
}
func (m *model) documentSpan(text []rune, start, end int, style lipgloss.Style) string {
	cursor := m.cursorOffset()
	if m.cursorActive() && cursor >= start && cursor < end {
		runeText := safe(string(text[cursor]))
		if runeText == "\n" {
			runeText = " "
		}
		middle := style.Reverse(true).Render(runeText)
		if text[cursor] == '\n' {
			middle += "\n"
		}
		return style.Render(safe(string(text[start:cursor]))) + middle + style.Render(safe(string(text[cursor+1:end])))
	}
	return style.Render(safe(string(text[start:end])))
}
func (m *model) generateAtCursor(siblings bool, options ...generationOptions) tea.Cmd {
	if !m.cursorActive() || m.pending {
		return nil
	}
	args := map[string]any{"node": m.currentID(), "offset": m.cursorOffset(), "branch": siblings}
	if len(options) > 0 {
		options[0].apply(args)
	}
	return m.send("continue", args)
}
func (m *model) generateDraft(siblings bool, options ...generationOptions) tea.Cmd {
	if m.pending || m.data.Busy {
		return nil
	}
	args := map[string]any{"node": m.editNode, "text": m.editor.Value(), "offset": textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column()), "branch": siblings}
	if len(options) > 0 {
		options[0].apply(args)
	}
	cmd := m.send("continue", args)
	if cmd != nil {
		m.editing = ""
		m.editor.Blur()
		m.section = 1
	}
	return cmd
}

// Transfer the cursor engine into an editable draft without resetting its
// position. Saved node text stays immutable until an explicit save/generation.
func (m *model) startInlineDraft(msg tea.Msg) tea.Cmd {
	if !m.cursorActive() || m.pending {
		return nil
	}
	m.openNotes(false)
	m.editing = "document"
	m.editNode = m.currentID()
	m.editor = m.navigator
	m.editor.SetVirtualCursor(true)
	plain := textarea.StyleState{Placeholder: dim, Selection: selectedStyle}
	m.editor.SetStyles(textarea.Styles{Focused: plain, Blurred: plain})
	focus := m.editor.Focus()
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	m.reflow()
	return tea.Batch(focus, cmd)
}
func documentInput(msg tea.KeyPressMsg) bool {
	if msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper|tea.ModMeta) != 0 {
		return false
	}
	return msg.Text != "" || msg.Code == tea.KeySpace || msg.Code == tea.KeyEnter || msg.Code == tea.KeyBackspace || msg.Code == tea.KeyDelete
}

// Fork copies the complete version. Cursor position is metadata, not a cut:
// generation alone consumes a prefix, and the original remains immutable.
func (m *model) forkDocument() tea.Cmd {
	if m.section == 3 {
		if args, ok := m.conversationTarget(); ok {
			return m.send("simulator.fork", args)
		}
		m.status = "Select a conversation to fork"
		return nil
	}
	if m.pending || m.data.Busy {
		return nil
	}
	id := m.currentID()
	args := map[string]any{"node": id}
	if m.editing == "document" {
		args["node"] = m.editNode
		args["text"] = m.editor.Value()
	} else if id == "" || m.targetRow().id != id {
		m.status = "Wait for the selected document to load"
		return m.previewTarget()
	}
	cmd := m.send("node.fork", args)
	if cmd != nil {
		m.editing = ""
		m.editor.Blur()
		m.commandDocument = ""
		m.enterLoom = true
		m.section = 1
	}
	return cmd
}

// Every explicit document-open route shares the same notes + editor state.
// Reuse the cursor engine so opening never inserts text or moves the cursor.
func (m *model) editDocumentWithNotes() tea.Cmd {
	if m.pending || m.data.Busy || m.data.Current == nil {
		return nil
	}
	if m.section != 1 && m.section != 2 {
		m.section = 1
	}
	m.focus = 1
	m.reflow()
	if m.editing == "document" {
		m.openNotes(false)
		return m.editor.Focus()
	}
	return m.startInlineDraft(nil)
}

func (m *model) openDocumentWithNotes() tea.Cmd {
	m.openNotes(false)
	m.focus = 1
	m.reflow()
	m.revealCursor()
	return nil
}
func (m *model) cancelEdit() tea.Cmd {
	m.dialog, m.editReturn = m.editReturn, nil
	m.editing = ""
	m.editor.Blur()
	m.focus = 1
	m.command.SetValue("")
	m.reflow()
	m.status = "Edit cancelled"
	return nil
}
func (m *model) documentActions() tea.Cmd {
	m.dialog = &dialog{kind: "document-actions", title: "Document", rows: []row{
		{id: "edit", label: "Edit here", preview: "Start a draft at the cursor. /save or CTRL+ENTER saves; /cancel or ESC returns."},
		{id: "notes", label: "Notes", preview: "Read or add separate notes for this document."},
	}}
	return nil
}

// Preview the selected version's own change, rather than inherited AI text.
// Use the same wrapping as the document so long lines and Unicode align.
func (m *model) revealVersionChange() {
	if m.data.Current == nil {
		return
	}
	text := []rune(m.currentText())
	offset := max(0, min(m.data.Current.ChangeOffset, len(text)))
	moveTextCursor(&m.navigator, offset)
	m.document.SetContent(m.renderDocument(m.document.Width()))
	row := strings.Count(ansi.Wrap(safe(string(text[:offset]))+"█", max(1, m.document.Width()), ""), "\n")
	m.document.SetYOffset(row)
}
