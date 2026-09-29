package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestActionPaletteUsesSmallVocabulary(t *testing.T) {
	for section := 0; section < 5; section++ {
		m := fixture()
		m.section = section
		m.focusCommand(true)
		m.command.SetValue("/")
		for _, a := range m.commandChoices() {
			if !m.paletteAvailable(a.id, "/") {
				t.Fatalf("legacy command leaked: %s", a.id)
			}
		}
		for _, hidden := range []string{"model", "workspace", "keys", "edit", "restart"} {
			m.command.SetValue("/" + hidden)
			if len(m.commandChoices()) == 0 && hidden != "edit" && !(section == 4 && hidden == "model") {
				t.Fatalf("legacy %s unreachable in %d", hidden, section)
			}
		}
	}
}
func TestOperationOverridesAreParsedWithoutMutatingDefaults(t *testing.T) {
	options, err := parseGenerationOptions(`/continue --visitor "A new question?" --model base --visitor-model guest --tokens 128 --turns 2 --eval "Voice"`, "continue")
	if err != nil || options.Model != "base" || options.VisitorModel != "guest" || options.Message != "A new question?" {
		t.Fatal(options, err)
	}
	for _, bad := range []string{"/continue 4", "/continue --count 4", "/continue --loops 2"} {
		if _, err := parseGenerationOptions(bad, "continue"); err == nil {
			t.Fatal(bad)
		}
	}
	m := simulatorFixture()
	m.selectSimulation("batch")
	options.Action = "continue"
	args, label, err := m.simulationLoomPlan(options)
	if err != nil || args["action"] != "continue" || args["count"] != nil || args["loops"] != nil || !strings.Contains(label, "2 selected conversations") {
		t.Fatal(args, label, err)
	}
	if m.simString("character_alias") == "base" {
		t.Fatal("operation changed config")
	}
}
func TestAlternativeSetsStayNestedAndTargetWholeGroup(t *testing.T) {
	m := simulatorFixture()
	m.simulation = nil
	m.data.SimulationRuns = nil
	for i := 0; i < 2; i++ {
		m.data.SimulationRuns = append(m.data.SimulationRuns, runSummary{ID: []string{"a", "b"}[i], Status: "complete", Count: 2, Conversations: []simulationConversation{{Index: 0}, {Index: 1}}, AlternativeGroup: "g", AlternativeIndex: i, AlternativeCount: 2})
	}
	rows := m.simulationRows()
	if rows[2].kind != "simulation-group" || rows[3].depth != 1 || rows[4].depth != 2 || rows[5].depth != 3 {
		t.Fatal(rows)
	}
	m.simSelection = &simulationSelection{Group: "g"}
	if len(m.selectedConversations()) != 4 {
		t.Fatal(m.selectedConversations())
	}
	args, label, err := m.simulationLoomPlan(generationOptions{Count: 3})
	if err != nil || len(args["targets"].([]conversationParent)) != 4 || !strings.Contains(label, "3 alternative sets of 4") {
		t.Fatal(args, label, err)
	}
	m.simSelection = &simulationSelection{Group: "g/1"}
	targets := m.selectedConversations()
	if len(targets) != 2 || targets[0].Run != "b" {
		t.Fatal(targets)
	}
	// A child explicitly replaces the parent's whole-set scope.
	m.toggleConversation("b", 1)
	if targets = m.selectedConversations(); len(targets) != 1 || targets[0].Conversation != 1 {
		t.Fatal(targets)
	}
}
func TestContinueAndLoomSendDifferentOperations(t *testing.T) {
	for _, action := range []string{"continue", "loom"} {
		m := simulatorFixture()
		m.selectSimulation("batch")
		req := captureCommand(t, m, func() tea.Cmd {
			m.focusCommand(true)
			m.command.SetValue("/" + action)
			return m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		})
		var got string
		json.Unmarshal(req.Args["action"], &got)
		if got != action {
			t.Fatal(got)
		}
		if action == "continue" && req.Args["count"] != nil {
			t.Fatal("continue count")
		}
	}
}
func TestConfigExposesWorkspaceAndBindings(t *testing.T) {
	for section := 0; section < 5; section++ {
		m := fixture()
		m.section = section
		m.openConfig()
		found := map[string]bool{}
		for _, r := range m.dialog.rows {
			found[r.id] = true
		}
		if !found["workspaces"] || !found["keys"] {
			t.Fatal(section, m.dialog)
		}
	}
}
func TestAddDoesNotRunEvaluation(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.addItem()
	if m.dialog == nil || m.dialog.kind != "eval-add-items" {
		t.Fatal(m.dialog)
	}
}
func TestDocumentSetShowsMembershipAndDispatchesGroup(t *testing.T) {
	m := fixture()
	m.section = 1
	m.data.DocumentSets = []documentSet{{ID: "set1", SetID: "logical", Members: []string{m.data.Nodes[0].ID}, Action: "loom"}}
	rows := m.rows()
	if rows[0].kind != "document-set" || rows[1].depth != 1 {
		t.Fatal(rows)
	}
	m.selected = 0
	req := captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Action: "continue"}) })
	var set string
	json.Unmarshal(req.Args["set"], &set)
	if set != "set1" {
		t.Fatal(req)
	}
}

func TestContinueKeepsExplicitSubsetThroughStreamEvents(t *testing.T) {
	m := simulatorFixture()
	m.selectLoomConversation(map[string]any{"run": "batch", "conversation": 1})
	m.conversationOpen = true
	m.gridSelection = 1
	captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Action: "continue"}) })
	data, _ := json.Marshal(m.simulation)
	m.apply(event{Type: "simulation", Data: data})
	targets := m.selectedConversations()
	if len(targets) != 1 || targets[0].Conversation != 1 || !m.conversationOpen {
		t.Fatal(targets, m.conversationOpen)
	}
}
func TestStreamingDoesNotStealGroupSelection(t *testing.T) {
	m := simulatorFixture()
	m.simulation = nil
	m.simSelection = &simulationSelection{Group: "g/1"}
	m.focus = 0
	m.apply(event{Type: "simulation", Data: json.RawMessage(`{"id":"background","status":"running","alternative_group":"g","alternative_index":0,"alternative_count":2,"conversations":[{"index":0}]}`)})
	if m.simSelection.Group != "g/1" || m.focus != 0 || m.simulation != nil {
		t.Fatal("background event changed user target")
	}
}
func TestCheckedDocumentsWinOverHoveredSet(t *testing.T) {
	m := fixture()
	m.section = 1
	n := m.data.Nodes[0]
	copy := n
	copy.ID = "other"
	m.data.Nodes = append(m.data.Nodes, copy)
	m.data.DocumentSets = []documentSet{{ID: "s", Members: []string{copy.ID}, Action: "loom"}}
	m.selected = 0
	m.branchSelection = map[string]bool{n.ID: true}
	args := m.documentGroupArgs()
	if args["set"] != nil || args["nodes"].([]string)[0] != n.ID {
		t.Fatal(args)
	}
}
func TestRemovedGroupLeavesStayReachable(t *testing.T) {
	m := fixture()
	m.section = 1
	m.data.DocumentSets = []documentSet{{ID: "s", Members: []string{m.data.Nodes[0].ID, "removed"}}}
	for _, r := range m.rows() {
		if r.kind == "document-set" {
			t.Fatal("ghost group")
		}
	}
	if len(m.rows()) == 0 || m.rows()[0].kind != "node" {
		t.Fatal("surviving document disappeared")
	}
}
func TestNotesPaletteCannotDeleteOwningDocument(t *testing.T) {
	m := fixture()
	m.section = 1
	m.openNotes(true)
	m.focusCommand(true)
	for _, command := range []string{"remove", "branch", "eval", "export"} {
		m.command.SetValue("/" + command)
		if len(m.commandChoices()) > 0 {
			t.Fatal(command, m.commandChoices())
		}
	}
	m.command.SetValue("/add")
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog == nil || m.dialog.kind != "note-new" {
		t.Fatal(m.dialog)
	}
}
func TestEvaluationRemoveAndUntrainAreDifferent(t *testing.T) {
	for _, operation := range []string{"remove", "untrain"} {
		m := evalFixture()
		m.section = 4
		m.evalCollection = "set"
		req := captureCommand(t, m, func() tea.Cmd { return m.evalAction(operation) })
		want := "evaluation.collection.remove"
		if operation == "untrain" {
			want = "evaluation.item.annotate"
		}
		if req.Command != want {
			t.Fatal(req.Command)
		}
	}
}
func TestContinueUpdatesCheckedDocumentHeads(t *testing.T) {
	m := fixture()
	m.branchSelection = map[string]bool{"old": true}
	m.pendingDocumentSelection = map[string]bool{"old": true}
	m.data.Nodes = []node{{ID: "old", DocumentID: "logical"}, {ID: "new", DocumentID: "logical"}}
	m.data.DocumentHeads = map[string]string{"logical": "new"}
	m.finishDocumentSelection()
	if !m.branchSelection["new"] || m.branchSelection["old"] {
		t.Fatal(m.branchSelection)
	}
}

func TestNewConversationControlStartsFreshLoom(t *testing.T) {
	m := simulatorFixture()
	m.selectSimulation("batch")
	m.focus = 0
	for i, r := range m.rows() {
		if r.kind == "sim-run" {
			m.selected = i
			break
		}
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.activate() })
	var action string
	json.Unmarshal(req.Args["action"], &action)
	if req.Command != "simulator.run" || action != "loom" || req.Args["run"] != nil || req.Args["targets"] != nil || m.simSelection != nil {
		t.Fatal(req, m.simSelection)
	}
}
