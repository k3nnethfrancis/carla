package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
)

type binding struct{ id, label, key, scope string }

func bindingCatalog() []binding {
	out := []binding{}
	for _, a := range allActions {
		out = append(out, binding{a.id, a.label, a.key, "panels"})
	}
	out = append(out,
		binding{"commands", "Open command list", "/", "panels"},
		binding{"commands.alt", "Open commands (alternate)", "ctrl+k", "panels"},
		binding{"remove.alt", "Remove (Backspace)", "backspace", "panels"},
		binding{"keep.alt", "Keep branch (alternate)", "", "panels"},
		binding{"select", "Select / deselect item", "space", "panels"},
		binding{"find", "Find passage", "ctrl+f", "panels"},
		binding{"keys", "Edit keybindings", "ctrl+n", "panels"},
		binding{"help", "Help", "?", "panels"},
		binding{"cancel", "Stop generation", "", "panels"},
		binding{"restart", "Restart Carla", "", "panels"},
		binding{"quit", "Exit Carla (/quit or /exit)", "ctrl+q", "global"},
		binding{"save", "Save draft", "ctrl+enter", "editor"},
		binding{"discard", "Discard draft", "", "editor"},
		binding{"edit.branch", "Branch current draft", "ctrl+b", "editor"},
		binding{"edit.review", "Review selection", "ctrl+u", "editor"},
		binding{"nav.next", "Next panel", "tab", "navigation"},
		binding{"nav.prev", "Previous panel", "shift+tab", "navigation"},
		binding{"nav.back", "Move outward / dismiss", "esc", "navigation"},
		binding{"nav.enter", "Open / move forward", "enter", "navigation"},
		binding{"nav.up", "Move up", "up", "navigation"},
		binding{"nav.down", "Move down", "down", "navigation"},
		binding{"nav.left", "Move left / collapse", "left", "navigation"},
		binding{"nav.right", "Move right / expand", "right", "navigation"},
	)
	return out
}
func bindingValue(values map[string]string, b binding) string {
	if key, ok := values[b.id]; ok {
		return key
	}
	return b.key
}
func (m *model) bindingKey(id string) string {
	for _, b := range bindingCatalog() {
		if b.id == id {
			return bindingValue(m.data.Bindings, b)
		}
	}
	return ""
}
func (m *model) keyLabel(id string) string {
	key := m.bindingKey(id)
	if key == "" {
		return "—"
	}
	return strings.ToUpper(key)
}
func (m *model) navigationKey(raw string) string {
	for _, b := range bindingCatalog() {
		if b.scope == "navigation" && raw == bindingValue(m.data.Bindings, b) {
			return b.id
		}
	}
	return "raw:" + raw
}
func (m *model) boundAction(raw, scope string) string {
	for _, b := range bindingCatalog() {
		if b.scope == scope && raw != "" && raw == bindingValue(m.data.Bindings, b) {
			return b.id
		}
	}
	return ""
}
func (m *model) openKeys() tea.Cmd {
	m.keyDraft = map[string]string{}
	for k, v := range m.data.Bindings {
		m.keyDraft[k] = v
	}
	m.keyCapture = false
	m.keySaving = false
	m.keyNotice = ""
	m.dialog = &dialog{kind: "keys", title: "Keybindings"}
	m.refreshKeys()
	return nil
}
func (m *model) refreshKeys() {
	d := m.dialog
	d.rows = nil
	for _, b := range bindingCatalog() {
		key := bindingValue(m.keyDraft, b)
		if key == "" {
			key = "unbound"
		}
		preview := b.scope + " · BACKSPACE unbind · CTRL+R restore default. Text entry keeps ordinary typing."
		if m.keyNotice != "" {
			preview = m.keyNotice
		}
		if m.keyCapture {
			preview = "Press a key combination. ESC cancels capture. CTRL+C remains emergency quit."
		}
		d.rows = append(d.rows, row{id: b.id, label: fmt.Sprintf("%-28s %s", b.label, strings.ToUpper(key)), preview: preview})
	}
}
func bindingConflict(values map[string]string, target binding, key string) string {
	if key == "/" && target.id != "commands" {
		return "/ is reserved for opening commands"
	}
	if key == "ctrl+c" {
		return "CTRL+C is reserved for emergency quit"
	}
	// Printable navigation/global bindings would swallow normal typing.
	if (target.scope == "navigation" || target.scope == "global" || target.scope == "editor") && !strings.Contains(key, "+") && len([]rune(key)) == 1 {
		return "Use a modifier or a navigation key for this action"
	}
	for _, b := range bindingCatalog() {
		if b.id == target.id || bindingValue(values, b) != key {
			continue
		}
		if b.scope == target.scope || b.scope == "global" || target.scope == "global" || b.scope == "navigation" || target.scope == "navigation" {
			return "Already assigned to " + b.label + "; unbind it first"
		}
	}
	return ""
}

// Fixed controls inside this editor provide a recovery path even after changing
// navigation. Nothing applies until Save succeeds in the Python settings store.
func (m *model) keysKey(msg tea.KeyPressMsg) tea.Cmd {
	raw := msg.String()
	d := m.dialog
	b := bindingCatalog()[d.index]
	if m.keyCapture {
		if raw == "esc" {
			m.keyCapture = false
			m.keyNotice = ""
			m.refreshKeys()
			return nil
		}
		if conflict := bindingConflict(m.keyDraft, b, raw); conflict != "" {
			m.keyCapture = false
			m.keyNotice = conflict
			m.refreshKeys()
			return nil
		}
		m.keyDraft[b.id] = raw
		m.keyCapture = false
		m.keyNotice = "Changed · CTRL+S saves all bindings"
		m.refreshKeys()
		return nil
	}
	switch raw {
	case "esc":
		m.dialog = nil
		m.keyDraft = nil
	case "enter":
		m.keyCapture = true
		m.keyNotice = ""
		m.refreshKeys()
	case "up", "down":
		step := 1
		if raw == "up" {
			step = -1
		}
		d.index = (d.index + step + len(d.rows)) % len(d.rows)
		m.keyNotice = ""
		m.refreshKeys()
	case "backspace", "delete":
		m.keyDraft[b.id] = ""
		m.keyNotice = "Unbound · CTRL+S saves"
		m.refreshKeys()
	case "ctrl+r":
		if conflict := bindingConflict(m.keyDraft, b, b.key); b.key != "" && conflict != "" {
			m.keyNotice = conflict
		} else {
			delete(m.keyDraft, b.id)
			m.keyNotice = "Default restored · CTRL+S saves"
		}
		m.refreshKeys()
	case "ctrl+s":
		if m.pending {
			return nil
		}
		m.keySaving = true
		return m.send("bindings.save", map[string]any{"bindings": m.keyDraft})
	}
	return nil
}

func navigationMessage(key string, original tea.KeyPressMsg) tea.KeyPressMsg {
	codes := map[string]rune{"nav.up": tea.KeyUp, "nav.down": tea.KeyDown, "nav.left": tea.KeyLeft, "nav.right": tea.KeyRight}
	if code, ok := codes[key]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	// Prevent a removed arrow binding from still scrolling through viewport defaults.
	if original.Code == tea.KeyUp || original.Code == tea.KeyDown || original.Code == tea.KeyLeft || original.Code == tea.KeyRight {
		return tea.KeyPressMsg{}
	}
	return original
}
