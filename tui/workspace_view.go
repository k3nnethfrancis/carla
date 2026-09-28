package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Location is UI-only workspace state. Never restore editing, checked targets,
// dialogs or command input: reopening a view must not authorize an operation.
type workspaceView struct {
	Collection string `json:"collection,omitempty"`
	Section    int    `json:"section"`
	Row        string `json:"row"`
}

func (m *model) workspaceView() workspaceView {
	row := m.targetRow().id
	if m.notesOpen {
		row = m.currentID()
	}
	return workspaceView{Section: m.section, Row: row, Collection: m.evalCollection}
}

func (m *model) saveWorkspaceView() {
	if m.data.Workspace.Path == "" || m.restoringView {
		return
	}
	data, err := json.Marshal(m.workspaceView())
	if err != nil {
		return
	}
	path := filepath.Join(m.data.Workspace.Path, "view-state.json")
	// Same atomic replacement as command history; no project or trace mutation.
	if err = os.WriteFile(path+".tmp", data, 0600); err == nil {
		err = os.Rename(path+".tmp", path)
	}
	if err != nil {
		m.status = "Could not save view: " + err.Error()
	}
}

func (m *model) restoreWorkspaceView() tea.Cmd {
	defer m.reflow()
	m.focus, m.sectionFocus = 3, false
	m.command.Focus()
	var saved workspaceView
	data, err := os.ReadFile(filepath.Join(m.data.Workspace.Path, "view-state.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil || saved.Section < 0 || saved.Section > 4 {
		return nil
	}
	m.section, m.selected = saved.Section, 0
	m.evalCollection = saved.Collection
	for _, source := range m.sources {
		if strings.HasPrefix(saved.Row, source.Key+":") {
			m.expanded[source.Key] = true
		}
	}
	// Trees start expanded; locate by stable identity, never an old row index.
	for i, row := range m.rows() {
		if row.id != saved.Row {
			continue
		}
		m.selected = i
		m.reflow()
		switch row.kind {
		case "conversation":
			if args, ok := m.conversationTarget(); ok {
				m.restoringView = true
				return m.send("simulator.open", args)
			}
		case "simulation":
			m.restoringView = true
			return m.send("simulator.open", map[string]any{"run": row.id})
		default:
			return m.previewTarget()
		}
	}
	m.reflow()
	return nil
}
