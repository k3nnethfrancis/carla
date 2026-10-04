package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"testing"
)

func TestNestedSimulatorScopesRetainContainmentAndContinueTargets(t *testing.T) {
	m := simulatorFixture()
	m.simulation = nil
	m.data.SimulationRuns = nil
	leaf := func(run string, i int) actionScope {
		return actionScope{Kind: "conversation", Run: run, Conversation: i}
	}
	tree := actionScope{Kind: "set", ID: "g/0/s", Children: []actionScope{
		{Kind: "set", ID: "g/0/s.0", Children: []actionScope{leaf("a", 0), leaf("a", 1)}},
		{Kind: "set", ID: "g/0/s.1", Children: []actionScope{leaf("b", 0), leaf("b", 1)}},
	}}
	for _, id := range []string{"a", "b"} {
		m.data.SimulationRuns = append(m.data.SimulationRuns, runSummary{ID: id, Count: 2, Status: "complete", AlternativeGroup: "g", AlternativeCount: 1, AlternativeScope: &tree, Conversations: []simulationConversation{{Index: 0}, {Index: 1}}})
	}
	m.simSelection = &simulationSelection{Group: "g/0/s"}
	args, _, err := m.simulationLoomPlan(generationOptions{Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	scope := args["scope"].(actionScope)
	if len(scope.Children) != 2 || len(scope.Children[0].Children) != 2 {
		t.Fatal(scope)
	}
	rows := m.simulationRows()
	leafCount := 0
	for _, r := range rows {
		if r.kind == "conversation" {
			leafCount++
		}
	}
	if leafCount != 4 {
		t.Fatal(rows)
	}
	m.toggleConversation("b", 1)
	args, _, err = m.simulationLoomPlan(generationOptions{})
	if err != nil || args["scope"].(actionScope).Kind != "conversation" || args["scope"].(actionScope).Run != "b" {
		t.Fatal(args, err)
	}
}

func TestSimulatorBranchDispatchesRecursiveScope(t *testing.T) {
	m := simulatorFixture()
	m.selectSimulation("batch")
	req := captureCommand(t, m, func() tea.Cmd { return m.forkDocument() })
	var scope actionScope
	if err := json.Unmarshal(req.Args["scope"], &scope); err != nil {
		t.Fatal(err)
	}
	if req.Command != "simulator.fork" || scope.Kind != "set" || len(scope.Children) != 2 {
		t.Fatal(req)
	}
}

func TestNestedSubsetsOfOneRunKeepTheirOwnRows(t *testing.T) {
	m := simulatorFixture()
	m.simulation = nil
	tree := actionScope{Kind: "set", ID: "g/0/s", Children: []actionScope{
		{Kind: "set", ID: "g/0/a", Children: []actionScope{{Kind: "conversation", Run: "r", Conversation: 0}}},
		{Kind: "set", ID: "g/0/b", Children: []actionScope{{Kind: "conversation", Run: "r", Conversation: 1}}},
	}}
	m.data.SimulationRuns = []runSummary{{ID: "r", Count: 2, AlternativeGroup: "g", AlternativeCount: 1, AlternativeScope: &tree, Conversations: []simulationConversation{{Index: 0}, {Index: 1}}}}
	rows := m.simulationRows()
	setIDs := map[string]bool{}
	leafIDs := map[string]int{}
	for _, r := range rows {
		if r.kind == "simulation-group" {
			setIDs[r.id] = true
		}
		if r.kind == "conversation" {
			leafIDs[r.id]++
		}
	}
	if !setIDs["g/0/a"] || !setIDs["g/0/b"] || len(leafIDs) != 2 {
		t.Fatal(rows)
	}
	for _, n := range leafIDs {
		if n != 1 {
			t.Fatal(rows)
		}
	}
}

func TestGroupGridDisplaysAllRunsAndOpensExactLeaf(t *testing.T) {
	m := simulatorFixture()
	m.awaitingSimulation = true // This stream follows an explicit run request.
	m.simulation = nil
	m.data.SimulationRuns = nil
	for i, id := range []string{"a", "b"} {
		run := simulationRun{ID: id, AlternativeGroup: "g", AlternativeIndex: i, AlternativeCount: 2, Conversations: []simulationConversation{{Index: 0, Status: "complete"}, {Index: 1, Status: "complete"}}}
		raw, _ := json.Marshal(run)
		m.apply(event{Type: "simulation", Data: raw})
	}
	m.gridGroup = "g"
	m.loomGrid = true
	m.focus = 1
	if len(m.gridItems()) != 4 {
		t.Fatal(m.gridItems())
	}
	if cmd := m.openGridTile(3); cmd != nil {
		t.Fatal("cached tile should open immediately")
	}
	if m.simulation.ID != "b" || m.gridSelection != 1 || len(m.selectedConversations()) != 1 {
		t.Fatal(m.simulation, m.gridSelection, m.selectedConversations())
	}
	if !m.simulatorBack() || !m.gridVisible() || len(m.gridItems()) != 4 || m.gridSelection != 3 {
		t.Fatal("back lost group grid")
	}
	// Stream deltas for a non-opened run must update its own tile, once.
	m.simulationViews["a"].Conversations[0].Turns = []simulationTurn{{Role: "character", Text: "A"}}
	raw := json.RawMessage(`{"run":"a","conversation":0,"turn":0,"text":"B"}`)
	m.apply(event{Type: "simulation.token", Data: raw})
	if m.simulationViews["a"].Conversations[0].Turns[0].Text != "AB" {
		t.Fatal(m.gridItems())
	}
}

func TestOpenActiveReplacesPreviousGridGroup(t *testing.T) {
	m := simulatorFixture()
	m.gridGroup = "old-group"
	m.data.Busy = true
	m.activeSimulation = &simulationRun{ID: "single", Conversations: []simulationConversation{{Index: 0}}}
	m.openActive()
	if m.gridGroup != "" || !m.conversationOpen {
		t.Fatal("single active conversation inherited old grid", m.gridGroup)
	}
	m.activeSimulation = &simulationRun{ID: "new", AlternativeGroup: "new-group", Conversations: []simulationConversation{{Index: 0}}}
	m.openActive()
	if m.gridGroup != "new-group" || m.conversationOpen || !m.loomGrid {
		t.Fatal("active alternative should open its group grid", m.gridGroup)
	}
}
