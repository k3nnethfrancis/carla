package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestWorkspaceViewRestoresLocationWithoutActions(t *testing.T) {
	for _, section := range []int{0, 1, 2} {
		m := fixture()
		m.data.Workspace.Path = t.TempDir()
		m.data.Nodes[0].Kept = true
		m.section = section
		rowID := m.currentID()
		if section == 0 {
			rowID = "gunkel:paths"
		}
		for i, r := range m.rows() {
			if r.id == rowID {
				m.selected = i
			}
		}
		m.saveWorkspaceView()
		m.section, m.selected, m.focus = 0, 0, 3
		m.expanded = map[string]bool{}
		m.restoreWorkspaceView()
		if m.section != section || m.targetRow().id != rowID || m.sectionFocus || m.focus != 3 || !m.command.Focused() {
			t.Fatalf("section %d restored to %d/%s focus %d", section, m.section, m.targetRow().id, m.focus)
		}
		if m.editing != "" || m.loomConversation != nil {
			t.Fatal("restoration activated editing/selection")
		}
	}
}

func TestWorkspaceViewRestoresConversationAtCommandBar(t *testing.T) {
	m := simulatorFixture()
	m.data.Workspace.Path = t.TempDir()
	m.selected = 4
	m.saveWorkspaceView()
	m.focus, m.sectionFocus = 3, false
	if cmd := m.restoreWorkspaceView(); cmd == nil {
		t.Fatal("missing conversation load")
	}
	run := *m.simulation
	run.Opened = true
	index := 1
	run.OpenConversation = &index
	data, _ := json.Marshal(run)
	m.apply(event{Type: "simulation", Data: data})
	if m.sectionFocus || m.focus != 3 || !m.command.Focused() || m.gridSelection != 1 || !m.conversationOpen || m.loomConversation != nil {
		t.Fatal("conversation restore must preserve command focus without checking a Loom target")
	}
}

func TestWorkspaceViewPersistsNavigationAndHandlesStaleFile(t *testing.T) {
	m := fixture()
	m.data.Workspace.Path = t.TempDir()
	m.width, m.height = 120, 36
	m.section, m.focus = 0, 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	data, err := os.ReadFile(filepath.Join(m.data.Workspace.Path, "view-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved workspaceView
	json.Unmarshal(data, &saved)
	if saved.Row != "gunkel:paths" {
		t.Fatalf("saved %s", data)
	}
	for _, content := range []string{`{"section":99}`, `broken`, `{"section":1,"row":"deleted"}`} {
		os.WriteFile(filepath.Join(m.data.Workspace.Path, "view-state.json"), []byte(content), 0600)
		m.restoreWorkspaceView()
		if m.section < 0 || m.section > 4 || m.sectionFocus || !m.command.Focused() {
			t.Fatal("invalid startup location")
		}
	}
}

func TestWorkspaceViewRestoresEvaluation(t *testing.T) {
	m := fixture()
	m.data.Workspace.Path = t.TempDir()
	m.data.Evaluations = []evaluationSummary{{ID: "review", Title: "Reviewed trace"}}
	m.section = 4
	for i, r := range m.rows() {
		if r.id == "review" {
			m.selected = i
		}
	}
	m.saveWorkspaceView()
	m.section, m.selected = 0, 0
	if m.restoreWorkspaceView() == nil || m.targetRow().id != "review" || m.sectionFocus || !m.command.Focused() {
		t.Fatal("evaluation was not restored")
	}
}
