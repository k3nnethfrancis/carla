package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func simulatorFixture() *model {
	m := fixture()
	m.width, m.height, m.section, m.focus = 120, 36, 3, 1
	m.simulation = &simulationRun{ID: "batch", Status: "complete", Conversations: []simulationConversation{
		{Index: 0, Status: "complete", Turns: []simulationTurn{{Role: "user", Text: "First opening"}, {Role: "character", Text: "ONLY FIRST"}}},
		{Index: 1, Status: "complete", Turns: []simulationTurn{{Role: "user", Text: "Second opening"}, {Role: "character", Text: "ONLY SECOND", Monitor: monitorResult{Status: "complete", Scores: map[string]float64{"looping": .7}}}}},
	}}
	m.loomGrid = true
	m.gridPinned = true
	return m
}
func TestConversationViewerIsolationAndEscapePath(t *testing.T) {
	m := simulatorFixture()
	m.openGridTile(1)
	text := ansi.Strip(m.conversationDocument(70))
	if strings.Contains(text, "ONLY FIRST") || !strings.Contains(text, "ONLY SECOND") || !strings.Contains(text, "// policy") {
		t.Fatal(text)
	}
	m.reflow()
	m.document.GotoBottom()
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	m.Update(esc)
	if m.conversationOpen || !m.gridVisible() || m.focus != 1 || m.gridSelection != 1 {
		t.Fatal("viewer did not return to selected grid tile")
	}
	m.Update(esc)
	if m.focus != 0 || m.sectionFocus {
		t.Fatal("grid did not return to list")
	}
	m.Update(esc)
	if !m.sectionFocus {
		t.Fatal("list did not return to tabs")
	}
	m.sectionFocus = false
	m.focus = 1
	m.openGridTile(0)
	if strings.Contains(m.conversationDocument(70), "ONLY SECOND") {
		t.Fatal("second conversation leaked")
	}
}
func TestConversationTreeAndTargets(t *testing.T) {
	m := simulatorFixture()
	m.data.SimulationRuns = []runSummary{
		{ID: "batch", Count: 2, Status: "complete", Conversations: m.simulation.Conversations},
		{ID: "fork", Count: 1, Status: "draft", Parent: &conversationParent{Run: "batch", Conversation: 1}},
	}
	rows := m.rows()
	if len(rows) != 6 || rows[2].kind != "simulation" || rows[5].depth != 2 {
		t.Fatal(rows)
	}
	m.focus = 0
	m.selected = 4
	args, ok := m.conversationTarget()
	if !ok || args["run"] != "batch" || args["conversation"] != 1 {
		t.Fatal(args)
	}
	m.conversationOpen = true
	m.gridSelection = 0
	args, _ = m.conversationTarget()
	if args["conversation"] != 1 {
		t.Fatal("viewer overrode list target", args)
	}
	m.focus = 1
	m.conversationOpen = false
	m.gridSelection = 1
	m.focusCommand(true)
	args, _ = m.conversationTarget()
	if args["conversation"] != 1 {
		t.Fatal("slash lost grid target", args)
	}
	m.focus = 0
	m.selected = 2
	m.conversationArrow("left")
	if len(m.rows()) != 3 {
		t.Fatal("collapsed descendants remained visible", m.rows())
	}
	m.conversationArrow("right")
	if len(m.rows()) != 6 {
		t.Fatal("expand lost descendants")
	}
}
func TestOpenedConversationAndUpdatesKeepViewer(t *testing.T) {
	m := simulatorFixture()
	data := json.RawMessage(`{"id":"batch","status":"running","opened":true,"open_conversation":1,"conversations":[{"index":0,"turns":[]},{"index":1,"turns":[{"role":"character","text":"SECOND"}]}]}`)
	m.apply(event{Type: "simulation", Data: data})
	if !m.conversationOpen || m.gridVisible() || m.gridSelection != 1 {
		t.Fatal("open not scoped")
	}
	data = json.RawMessage(strings.Replace(strings.Replace(string(data), `"opened":true,`, "", 1), `"open_conversation":1,`, "", 1))
	m.apply(event{Type: "simulation", Data: data})
	if !m.conversationOpen || m.gridSelection != 1 {
		t.Fatal("snapshot moved viewer")
	}
}
func TestConversationEditDraftAndCustomVisitor(t *testing.T) {
	m := simulatorFixture()
	m.openGridTile(1)
	m.editConversation(false)
	if m.dialog.kind != "conversation-edit" || len(m.dialog.rows) != 2 {
		t.Fatal("no turn picker")
	}
	m.dialog.index = 1
	m.submitDialog()
	if m.editing != "conversation" || m.editor.Value() != "ONLY SECOND" {
		t.Fatal("wrong edit target")
	}
	m.cancelEdit()
	if !m.conversationOpen {
		t.Fatal("cancel left viewer")
	}
	m.editConversation(true)
	if m.conversationEdit != -1 || m.editing != "conversation" || m.editor.Value() != "" {
		t.Fatal("visitor draft failed")
	}
}

func TestNewLoomSelectsItsTreeRowOnce(t *testing.T) {
	m := fixture()
	m.section = 3
	m.selected = 1
	data := json.RawMessage(`{"id":"new-run","status":"running","conversations":[{"index":0},{"index":1}]}`)
	m.apply(event{Type: "simulation", Data: data})
	if m.targetRow().id != "new-run" || m.collapsed["new-run"] {
		t.Fatal("new run not selected/revealed", m.targetRow())
	}
	m.selected = 0
	m.apply(event{Type: "simulation", Data: data})
	if m.selected != 0 {
		t.Fatal("stream update stole navigation")
	}
}
