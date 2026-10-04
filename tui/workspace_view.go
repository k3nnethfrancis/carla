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
	EvalArea   string `json:"eval_area,omitempty"`
	Collection string `json:"collection,omitempty"`
	Section    int    `json:"section"`
	Row        string `json:"row"`
}

func (m *model) workspaceView() workspaceView {
	row := m.targetRow().id
	if m.notesOpen {
		row = m.currentID()
	}
	return workspaceView{Section: m.section, Row: row, Collection: m.evalCollection, EvalArea: m.evalArea}
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
	// Start closed, then reveal only the ancestry needed for the saved document.
	// Use the real tree projection so Loom wrappers and nested sets count too.
	tree := *m
	tree.section, tree.filter, tree.collapsed = 1, "", nil
	trees := map[int][]row{1: tree.branchRows(), 3: tree.simulationRows()}
	m.collapsed = map[string]bool{}
	for _, rows := range trees {
		for _, row := range rows {
			m.collapsed[row.id] = true
		}
	}
	var saved workspaceView
	data, err := os.ReadFile(filepath.Join(m.data.Workspace.Path, "view-state.json"))
	if err != nil || json.Unmarshal(data, &saved) != nil || saved.Section < 0 || saved.Section > 4 {
		return nil
	}
	m.section, m.selected = saved.Section, 0
	if rows, ok := trees[saved.Section]; ok {
		for i, row := range rows {
			if row.id != saved.Row {
				continue
			}
			depth := row.depth
			for j := i - 1; j >= 0 && depth > 0; j-- {
				if rows[j].depth < depth {
					m.collapsed[rows[j].id] = false
					depth = rows[j].depth
				}
			}
			break
		}
	}
	m.evalCollection = saved.Collection
	m.evalArea = saved.EvalArea
	if m.evalCollection != "" {
		m.evalArea = "data"
	}
	if m.evalArea != "data" && m.evalArea != "policies" && m.evalArea != "runs" {
		m.evalArea = ""
	}
	for _, source := range m.sources {
		if strings.HasPrefix(saved.Row, source.Key+":") {
			m.expanded[source.Key] = true
		}
	}
	// Locate by stable identity, never an old row index.
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
