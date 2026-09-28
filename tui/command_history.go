package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Command history belongs to the workspace but never enters model context.
func (m *model) loadCommandHistory() {
	m.commandHistory = nil
	m.historyPosition = 0
	m.historyDraft = ""
	data, err := os.ReadFile(filepath.Join(m.data.Workspace.Path, "command-history.json"))
	if err == nil {
		_ = json.Unmarshal(data, &m.commandHistory)
	}
}
func (m *model) recordCommand(a action) {
	fields := strings.Fields(m.command.Value())
	command := "/" + commandName(a)
	if len(fields) > 1 {
		command += " " + strings.Join(fields[1:], " ")
	}
	n := len(m.commandHistory)
	if n == 0 || m.commandHistory[n-1] != command {
		m.commandHistory = append(m.commandHistory, command)
	}
	m.historyPosition = 0
	m.historyDraft = ""
	// Atomic replacement prevents a restart from observing a partial history.
	if m.data.Workspace.Path == "" {
		return
	}
	data, _ := json.MarshalIndent(m.commandHistory, "", "  ")
	path := filepath.Join(m.data.Workspace.Path, "command-history.json")
	if err := os.WriteFile(path+".tmp", data, 0600); err == nil {
		_ = os.Rename(path+".tmp", path)
	}
}
func (m *model) browseCommandHistory(step int) {
	if len(m.commandHistory) == 0 {
		return
	}
	if m.historyPosition == 0 {
		m.historyDraft = m.command.Value()
	}
	m.historyPosition = max(0, min(len(m.commandHistory), m.historyPosition+step))
	value := m.historyDraft
	if m.historyPosition > 0 {
		value = m.commandHistory[len(m.commandHistory)-m.historyPosition]
	}
	m.command.SetValue(value)
	m.command.CursorEnd()
	m.commandIndex = 0
	m.reflow()
}
func (m *model) commandHints() []string {
	if m.focus != 3 {
		return nil
	}
	fields := strings.Fields(m.command.Value())
	if len(fields) == 0 {
		return nil
	}
	switch fields[0] {
	case "/eval":
		return []string{"Run the active or named evaluation on selected items", "[evaluation-name] · --train-on-pass true|false"}
	case "/loom", "/continue", "/generate", "/run", "/grow", "/simulate":
		if m.section == 3 {
			return []string{"[alternatives] · --tokens N|Max · --turns N · --msg \"text\" · --eval name · --loops N", "Checked conversation or fresh setup · /clear starts fresh"}
		}
		return []string{"[alternatives] · --tokens N|Max · --eval name · --loops N", "Selected document → Branches · bare /loom = one continuation"}
	}
	return nil
}
