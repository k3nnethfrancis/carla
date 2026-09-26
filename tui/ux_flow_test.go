package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestPalettePreservesDraftAndDialog(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 1
	m.focus = 1
	m.beginEdit("document")
	m.editor.SetValue("An unsaved original.")
	m.editor.CursorEnd()
	text, offset := m.editor.Value(), textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column())
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.editing != "document" || m.editor.Value() != text || m.focus != 1 || textOffset(m.editor.Value(), m.editor.Line(), m.editor.Column()) != offset {
		t.Fatal("palette changed draft or cursor")
	}
	m.cancelEdit()
	m.openGrowConfig()
	parent := m.dialog
	parent.index = 6
	m.submitDialog()
	child := m.dialog
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != child || m.dialog.parent != parent {
		t.Fatal("palette lost nested config")
	}
}

func TestSettingsSaveReturnsToParentAndRefreshes(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 3
	m.data.SimulatorConfig = map[string]any{"character_alias": "base", "turns": float64(2)}
	m.openSimulatorConfig()
	parent := m.dialog
	parent.index = 1
	m.submitDialog()
	m.submitDialog() // model selection queues the same backend command as /model
	if m.dialog != parent || parent.index != 1 {
		t.Fatal("save closed parent")
	}
	m.pending = false
	m.data.SimulatorConfig["character_alias"] = "changed"
	m.refreshConfig()
	if m.dialog.index != 1 || !strings.Contains(m.dialog.rows[1].label, "changed") {
		t.Fatal("parent values stale")
	}
}

func TestPagePositionsAndFilterRestore(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 1
	m.focus = 1
	second := *m.data.Current
	second.ID = "second"
	second.Text = "first\nsecond\nthird"
	m.data.Nodes = append(m.data.Nodes, second)
	m.data.Current = &second
	m.selected = 1
	m.reflow()
	moveTextCursor(&m.navigator, 8)
	before := m.cursorOffset()
	m.switchSection(3)
	m.switchSection(1)
	if m.selected != 1 || m.focus != 1 || m.cursorOffset() != before {
		t.Fatalf("lost position: %d %d %d", m.selected, m.focus, m.cursorOffset())
	}
	m.perform("find")
	if m.section != 1 || !m.searching {
		t.Fatal("find navigated away")
	}
}

func TestBatchRemovalAndFilteredDocumentPicker(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 2
	m.data.Nodes[0].Kept = true
	second := m.data.Nodes[0]
	second.ID = "second"
	second.Title = "Another"
	m.data.Nodes = append(m.data.Nodes, second)
	m.branchSelection = map[string]bool{second.ID: true, m.data.Nodes[0].ID: true}
	m.openSelectionActions()
	if m.dialog.rows[1].id != "remove" || m.collectionCount() != 2 {
		t.Fatal("wrong anthology batch action")
	}
	m.dialog = nil
	m.data.SimulatorConfig = map[string]any{"documents": []any{second.ID}}
	m.openSimulatorConfig()
	m.submitDialog()
	m.filterDialog(tea.KeyPressMsg{Code: 'A', Text: "Another"})
	if len(m.dialog.rows) != 1 || m.dialog.rows[0].id != "second" {
		t.Fatal("document picker failed filtering")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if m.dialog.args["selected"].(map[string]bool)[second.ID] {
		t.Fatal("filtered toggle wrong")
	}
}

func TestBusyBrowsingAndSimulationDoesNotStealFocus(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 1
	m.focus = 0
	m.data.Busy = true
	m.command.SetValue("/branches")
	m.focus = 3
	if len(m.commandChoices()) != 1 {
		t.Fatal("busy navigation command missing")
	}
	m.command.SetValue("/run")
	for _, a := range m.commandChoices() {
		if a.id == "run" {
			t.Fatal("mutation available")
		}
	}
	old := &simulationRun{ID: "old", Opened: true}
	m.simulation = old
	data, _ := json.Marshal(simulationRun{ID: "active", Status: "running", Conversations: []simulationConversation{{Turns: []simulationTurn{{Role: "character"}}}}})
	m.apply(event{Type: "simulation", Data: data})
	m.apply(event{Type: "simulation.token", Data: json.RawMessage(`{"run":"active","conversation":0,"turn":0,"text":"hello"}`)})
	if m.section != 1 || m.simulation != old || m.activeSimulation.Conversations[0].Turns[0].Text != "hello" {
		t.Fatal("background overwrote viewed page/run")
	}
	m.openActive()
	if m.section != 3 || m.simulation.ID != "active" {
		t.Fatal("active jump failed")
	}
}

func TestIntentSearchAndScopedModel(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 3
	m.focus = 3
	m.command.SetValue("/temperature")
	choices := m.commandChoices()
	if len(choices) != 2 {
		t.Fatalf("sampling matches: %v", choices)
	}
	m.command.SetValue("/model visitor")
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog.kind != "sim-model" || m.dialog.args["field"] != "visitor_alias" {
		t.Fatal("model command changed Loom")
	}
	m.dialog = nil
	m.command.SetValue("/")
	seen := map[string]bool{}
	for _, a := range m.commandChoices() {
		if seen[a.id] {
			t.Fatal("duplicate")
		}
		seen[a.id] = true
	}
	if seen["simulate"] || seen["sim-config"] || seen["settings"] {
		t.Fatal("duplicate aliases exposed")
	}
}

func TestDeletedPageItemFallsBackAndFindLeavesNotes(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.section = 3
	m.pages[1] = &pagePosition{rowID: "deleted", nodeID: "deleted", index: 1, notes: true, cursor: 900, scroll: 900}
	m.switchSection(1)
	if m.notesOpen || m.selected != 0 {
		t.Fatal("deleted document restored as notes")
	}
	m.pending = false
	m.notesOpen = true
	m.perform("find")
	if m.notesOpen || !m.searching || m.section != 1 {
		t.Fatal("find trapped in notes")
	}
}

func TestHelpReturnsToSuspendedPicker(t *testing.T) {
	m := fixture()
	m.width, m.height = 120, 36
	m.openGrowConfig()
	parent := m.dialog
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.command.SetValue("/help")
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != parent {
		t.Fatal("Help lost suspended configuration")
	}
}
