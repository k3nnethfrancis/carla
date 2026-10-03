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
		if m.editing != "" || m.simSelection != nil {
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
	if m.sectionFocus || m.focus != 3 || !m.command.Focused() || m.gridSelection != 1 || !m.conversationOpen || m.simSelection != nil {
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
	m.evalCollection = "set"
	m.data.EvaluationSets = []evaluationCollection{{ID: "set", Items: []evaluationSummary{{ID: "review", Title: "Reviewed trace"}}}}
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

func TestBranchRestoreOpensOnlySavedDocumentAncestry(t *testing.T) {
	for _, target := range []string{"leaf", "a", "root", "deleted", ""} {
		m := fixture()
		m.data.Workspace.Path = t.TempDir()
		m.section = 1
		m.data.Nodes = []node{
			{ID: "root"}, {ID: "a", Parent: "root"}, {ID: "leaf", Parent: "a"},
			{ID: "sibling", Parent: "root"}, {ID: "other"}, {ID: "other-child", Parent: "other"},
		}
		m.data.DocumentSets = []documentSet{{ID: "set", Action: "loom", OperationID: "op", Members: []string{"a"}}}
		if target != "" {
			data, _ := json.Marshal(workspaceView{Section: 1, Row: target})
			os.WriteFile(filepath.Join(m.data.Workspace.Path, "view-state.json"), data, 0600)
		}
		m.restoreWorkspaceView()
		if !m.collapsed["other"] || !m.collapsed["sibling"] || !m.collapsed["leaf"] {
			t.Fatal("unrelated branches or focused document expanded", target, m.collapsed)
		}
		wantOpen := map[string]bool{}
		if target == "a" || target == "leaf" {
			wantOpen["root"], wantOpen["loom:op"] = true, true
		}
		if target == "leaf" {
			wantOpen["a"] = true
		}
		for _, id := range []string{"root", "loom:op", "a"} {
			if m.collapsed[id] == wantOpen[id] {
				t.Fatal("wrong ancestor expansion", target, id, m.collapsed)
			}
		}
		if target != "" && target != "deleted" && m.targetRow().id != target {
			t.Fatal("last document not revealed", target)
		}
		if m.focus != 3 || !m.command.Focused() || len(m.branchSelection) != 0 {
			t.Fatal("restore changed interaction state")
		}
	}
}

func TestSimulatorRestoreOpensOnlySavedAncestry(t *testing.T) {
	for _, target := range []string{"", "deleted", "batch", "batch:0", "child:1", "choices", "choices/1", "alt:0"} {
		t.Run(target, func(t *testing.T) {
			m := simulatorFixture()
			m.data.Workspace.Path = t.TempDir()
			m.data.SimulationRuns = []runSummary{
				{ID: "batch", Count: 2, Conversations: m.simulation.Conversations},
				{ID: "child", Count: 2, Conversations: m.simulation.Conversations, Parent: &conversationParent{Run: "batch", Conversation: 0}},
				{ID: "other", Count: 2},
				{ID: "alt", Count: 1, Conversations: []simulationConversation{{Index: 0}}, AlternativeGroup: "choices", AlternativeCount: 2, AlternativeIndex: 1},
			}
			if target != "" {
				raw, _ := json.Marshal(workspaceView{Section: 3, Row: target})
				os.WriteFile(filepath.Join(m.data.Workspace.Path, "view-state.json"), raw, 0600)
			}
			m.restoreWorkspaceView()
			expected := map[string]bool{}
			switch target {
			case "batch:0":
				expected["batch"] = true
			case "child:1":
				expected["batch"], expected["batch:0"], expected["child"] = true, true, true
			case "choices/1":
				expected["choices"] = true
			case "alt:0":
				expected["choices"], expected["choices/1"] = true, true
			}
			for _, id := range []string{"batch", "batch:0", "child", "other", "choices", "choices/0", "choices/1"} {
				if m.collapsed[id] == expected[id] {
					t.Fatalf("%s: wrong expansion %s: %v", target, id, m.collapsed)
				}
			}
			if target != "" && target != "deleted" && m.targetRow().id != target {
				t.Fatal("saved row not revealed", m.targetRow())
			}
			if m.simSelection != nil || m.focus != 3 || !m.command.Focused() {
				t.Fatal("restore must not check targets or steal command focus")
			}
		})
	}
}

func TestRestoredLoomResponseDoesNotExpandChildren(t *testing.T) {
	m := simulatorFixture()
	m.data.Workspace.Path = t.TempDir()
	raw, _ := json.Marshal(workspaceView{Section: 3, Row: "batch"})
	os.WriteFile(filepath.Join(m.data.Workspace.Path, "view-state.json"), raw, 0600)
	if m.restoreWorkspaceView() == nil {
		t.Fatal("missing load")
	}
	run := *m.simulation
	run.Opened = true
	raw, _ = json.Marshal(run)
	m.apply(event{Type: "simulation", Data: raw})
	if !m.collapsed["batch"] || m.targetRow().id != "batch" || m.focus != 3 || m.simSelection != nil {
		t.Fatal("restored grid expanded or activated the tree")
	}
	// An explicitly opened grid still reveals the run during normal navigation.
	m.apply(event{Type: "simulation", Data: raw})
	if m.collapsed["batch"] {
		t.Fatal("normal open should still reveal run")
	}
}
