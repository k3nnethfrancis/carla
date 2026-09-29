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
	for _, bad := range []string{"/continue 4", "/continue --count 4"} {
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
	if err != nil || len(simulationScopeLeaves(args["scope"].(actionScope))) != 4 || !strings.Contains(label, "3 alternative sets of 4") {
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
	copy := m.data.Nodes[0]
	copy.ID = "second"
	m.data.Nodes = append(m.data.Nodes, copy)
	m.data.DocumentSets = []documentSet{{ID: "set1", SetID: "logical", Members: []string{m.data.Nodes[0].ID, copy.ID}, Action: "loom"}}
	rows := m.rows()
	if rows[0].kind != "document-set" || rows[1].depth != 1 {
		t.Fatal(rows)
	}
	m.selected = 0
	req := captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Action: "continue"}) })
	var scope actionScope
	json.Unmarshal(req.Args["scope"], &scope)
	if scope.ID != "set1" {
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
	if args["scope"].(actionScope).Node != n.ID {
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

// Fork opens the reader and previews ChangeOffset=0. That automatic position
// must not silently turn a subsequent Continue into an empty-prefix branch.
func TestDocumentContinueUsesWholePreviewUnlessCursorDeliberatelyMoved(t *testing.T) {
	for _, focus := range []int{0, 1, 3} {
		for _, action := range []string{"continue", "loom"} {
			m := fixture()
			m.section, m.width, m.height = 1, 120, 36
			m.reflow()
			m.revealVersionChange()
			m.focus = focus
			if focus == 3 {
				m.commandOrigin = &commandOrigin{focus: 1}
			}
			req := captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Action: action}) })
			if req.Args["offset"] != nil || req.Args["offsets"] != nil {
				t.Fatalf("%s from focus %d truncated automatic preview: %v", action, focus, req.Args)
			}
		}
	}
	m := fixture()
	m.section, m.width, m.height = 1, 120, 36
	m.reflow()
	m.revealVersionChange()
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m.focusCommand(true)
	req := captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Action: "continue"}) })
	var offset int
	json.Unmarshal(req.Args["offset"], &offset)
	if offset != 1 {
		t.Fatalf("deliberate cursor lost: %v", req.Args)
	}
}
func TestSingletonOperationVersionsStayInDocumentAncestry(t *testing.T) {
	m := fixture()
	m.section = 1
	parent := m.data.Nodes[0]
	for _, id := range []string{"fork", "continued", "continued-again"} {
		n := parent
		n.ID, n.Parent = id, parent.ID
		m.data.Nodes = append(m.data.Nodes, n)
		m.data.DocumentSets = append(m.data.DocumentSets, documentSet{ID: "set-" + id, SetID: "logical-" + id, Action: "continue", Members: []string{id}})
		parent = n
	}
	rows := m.branchRows()
	if len(rows) != 4 {
		t.Fatal(rows)
	}
	for i, r := range rows {
		if r.kind != "node" || r.depth != i {
			t.Fatalf("detached operation wrapper: %v", rows)
		}
	}
}
func TestContextualContinueUsesSameSelectedTargetsAsCommand(t *testing.T) {
	m := fixture()
	m.section, m.focus = 1, 0
	second := m.data.Nodes[0]
	second.ID = "selected"
	m.data.Nodes = append(m.data.Nodes, second)
	m.branchSelection = map[string]bool{second.ID: true}
	req := captureCommand(t, m, func() tea.Cmd { return m.contextualAction("continue") })
	var action string
	var scope actionScope
	json.Unmarshal(req.Args["action"], &action)
	json.Unmarshal(req.Args["scope"], &scope)
	if action != "continue" || scope.Node != second.ID {
		t.Fatal(req)
	}
}

func TestExplicitDocumentSetScopeSurvivesHoverAndNestedSets(t *testing.T) {
	m := fixture()
	m.section = 1
	m.focus = 0
	m.data.Nodes = []node{{ID: "a", DocumentID: "a"}, {ID: "b", DocumentID: "b"}, {ID: "c", DocumentID: "c"}}
	scope := actionScope{Kind: "set", Children: []actionScope{{Kind: "set", Children: []actionScope{{Kind: "document", Node: "a"}, {Kind: "document", Node: "b"}}}, {Kind: "document", Node: "c"}}}
	m.data.DocumentSets = []documentSet{{ID: "s", SetID: "s", Members: []string{"a", "b", "c"}, Scope: &scope}}
	m.branchSelection = map[string]bool{"set:s": true, "a": true, "b": true, "c": true}
	m.selected = 2
	got := m.documentGroupArgs()["scope"].(actionScope)
	if got.ID != "s" || got.Children[0].Kind != "set" || len(got.Children[0].Children) != 2 {
		t.Fatal(got)
	}
	rows := m.branchRows()
	found := false
	for _, r := range rows {
		if r.id == "s/scope/0" && r.kind == "document-set" {
			found = true
		}
	}
	if !found {
		t.Fatal(rows)
	}
	if len(m.documentSetMembers("s/scope/0")) != 2 {
		t.Fatal(m.documentSetMembers("s/scope/0"))
	}
}

func TestNestedDocumentContinueRetainsScopeOnRepeatAndRespectsNavigation(t *testing.T) {
	for _, navigate := range []bool{false, true} {
		m := fixture()
		m.section, m.focus = 1, 0
		m.data.Nodes = []node{{ID: "a", DocumentID: "a", Status: "complete"}, {ID: "b", DocumentID: "b", Status: "complete"}, {ID: "c", DocumentID: "c", Status: "complete"}}
		nested := actionScope{Kind: "set", Children: []actionScope{{Kind: "document", Node: "a"}, {Kind: "document", Node: "b"}}}
		whole := actionScope{Kind: "set", Children: []actionScope{nested, {Kind: "document", Node: "c"}}}
		m.data.DocumentSets = []documentSet{{ID: "old", SetID: "old", Members: []string{"a", "b", "c"}, Scope: &whole}}
		m.data.DocumentSetHeads = map[string]string{"old": "old"}
		m.branchSelection = map[string]bool{"set:old/scope/0": true, "a": true, "b": true}
		for i, r := range m.rows() {
			if r.id == "old/scope/0" {
				m.selected = i
			}
		}
		captureCommand(t, m, func() tea.Cmd { return m.loom(generationOptions{Action: "continue"}) })
		if navigate {
			for i, r := range m.rows() {
				if r.id == "c" {
					m.selected = i
				}
			}
		}
		next := m.data
		next.Nodes = append(append([]node(nil), m.data.Nodes...), node{ID: "a2", DocumentID: "a2", Parent: "a", Status: "complete"}, node{ID: "b2", DocumentID: "b2", Parent: "b", Status: "complete"})
		output := actionScope{Kind: "set", Children: []actionScope{{Kind: "document", Node: "a2"}, {Kind: "document", Node: "b2"}}}
		next.DocumentSets = append(append([]documentSet(nil), m.data.DocumentSets...), documentSet{ID: "result", SetID: "result", Members: []string{"a2", "b2"}, Scope: &output, SourceScope: &nested})
		next.DocumentSetHeads = map[string]string{"old": "old", "result": "result"}
		next.DocumentHeads = map[string]string{"a": "a", "b": "b", "c": "c", "a2": "a2", "b2": "b2"}
		next.Busy = false
		data, _ := json.Marshal(next)
		m.apply(event{Type: "state", Data: data})
		if !m.branchSelection["set:result"] || m.branchSelection["set:old/scope/0"] {
			t.Fatal(m.branchSelection)
		}
		got := m.documentGroupArgs()["scope"].(actionScope)
		if got.ID != "result" || len(got.Children) != 2 || got.Children[0].Node != "a2" {
			t.Fatal(got)
		}
		want := "result"
		if navigate {
			want = "c"
		}
		if m.targetRow().id != want {
			t.Fatalf("navigate %v: preview %s want %s", navigate, m.targetRow().id, want)
		}
	}
}

func TestAnthologySelectedSetBranchRoutesToBranches(t *testing.T) {
	m := fixture()
	m.section = 2
	m.focus = 0
	first := m.data.Nodes[0]
	first.Kept = true
	second := first
	second.ID = "second"
	m.data.Nodes = []node{first, second}
	m.branchSelection = map[string]bool{first.ID: true, second.ID: true}
	req := captureCommand(t, m, m.forkDocument)
	var scope actionScope
	json.Unmarshal(req.Args["scope"], &scope)
	if req.Command != "node.fork" || scope.Kind != "set" || len(scope.Children) != 2 || m.section != 1 {
		t.Fatal(req, m.section)
	}
}
