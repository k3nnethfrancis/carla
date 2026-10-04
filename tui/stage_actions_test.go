package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"testing"
)

func TestGenerationUnavailableInLibraryAndEvaluate(t *testing.T) {
	for _, section := range []int{0, 4} {
		for _, action := range []string{"continue", "loom", "branch", "generate", "grow"} {
			m := fixture()
			m.section = section
			if cmd := m.perform(action); cmd != nil {
				t.Fatalf("stage %d exposed %s", section, action)
			}
			m.focusCommand(true)
			m.command.SetValue("/" + action)
			for _, choice := range m.commandChoices() {
				if choice.id == "continue" || choice.id == "loom" || choice.id == "branch" {
					t.Fatalf("unavailable completion: %+v", choice)
				}
			}
		}
	}
}
func TestAnthologyLoomStartsSimulatorWithExactDocuments(t *testing.T) {
	m := fixture()
	m.section = 2
	m.data.Nodes[0].Kept = true
	m.simSelection = &simulationSelection{Run: "stale", All: true}
	req := captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Count: 2, Turns: 3, Tokens: 64}) })
	var docs []string
	json.Unmarshal(req.Args["documents"], &docs)
	if req.Command != "simulator.run" || len(docs) != 1 || docs[0] != m.data.Nodes[0].ID || m.section != 3 || req.Args["scope"] != nil {
		t.Fatal(req, m.section)
	}
}
func TestAnthologyContinueBranchesWithoutKeepingResult(t *testing.T) {
	m := fixture()
	m.section = 2
	m.data.Nodes[0].Kept = true
	req := captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Action: "continue"}) })
	var fork bool
	json.Unmarshal(req.Args["from_anthology"], &fork)
	if req.Command != "continue" || !fork || m.section != 1 {
		t.Fatal(req, m.section)
	}
}
func TestDeleteRemovesOutsideTextEditing(t *testing.T) {
	for _, focus := range []int{0, 1} {
		m := fixture()
		m.section = 1
		m.width, m.height = 120, 36
		m.focus = focus
		m.reflow()
		m.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
		if m.dialog != nil {
			t.Fatalf("focus %d: unselected delete opened action", focus)
		}
	}
	m := fixture()
	m.section = 1
	m.focus = 1
	m.width, m.height = 120, 36
	m.editing = "document"
	m.editor.SetValue("AB")
	m.editor.CursorStart()
	m.editor.Focus()
	m.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
	if m.dialog != nil || m.editor.Value() != "B" {
		t.Fatal("Delete escaped editor", m.editor.Value(), m.dialog)
	}
	m = fixture()
	m.data.Bindings = map[string]string{"remove": "ctrl+d"}
	if m.bindingKey("remove") != "ctrl+d" || m.boundAction("delete", "panels") != "" {
		t.Fatal("custom binding overwritten")
	}
}
func TestContinueAcceptsLoopsWithoutAlternativeCount(t *testing.T) {
	opt, err := parseGenerationOptions("/continue --tokens 64 --loops 3", "continue")
	if err != nil || opt.Loops != 3 || opt.Tokens != 64 || opt.Count != 0 {
		t.Fatal(opt, err)
	}
}
