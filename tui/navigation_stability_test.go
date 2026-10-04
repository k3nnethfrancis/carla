package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"testing"
)

func TestNavigationStability(t *testing.T) {
	t.Run("wheel", func(t *testing.T) {
		m := fixture()
		m.width, m.height, m.section, m.focus = 120, 36, 1, 0
		m.data.Nodes = append(m.data.Nodes, node{ID: "other", Title: "Other", Text: "other text"})
		before := m.targetRow().id
		_, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		if cmd == nil {
			t.Errorf("wheel row %s -> %s, document %s, preview requested=%v", before, m.targetRow().id, m.currentID(), cmd != nil)
		}
	})
	t.Run("preview_then_enter", func(t *testing.T) {
		m := simulatorFixture()
		m.focus, m.selected = 0, 3
		m.data.SimulationRuns = []runSummary{{ID: "batch", Count: 2, Conversations: m.simulation.Conversations}}
		preview := m.previewTarget()
		enter := m.activate()
		if preview == nil || enter != nil {
			t.Fatal("expected preview in flight")
		}
		run := *m.simulation
		index := 0
		run.Browsed = true
		run.OpenConversation = &index
		raw, _ := json.Marshal(run)
		cmd := m.apply(event{Type: "simulation", ID: m.previewRequest, Data: raw})
		if cmd == nil || m.simSelection == nil {
			t.Fatal("Enter lost while preview pending")
		}
	})
	t.Run("busy_selection", func(t *testing.T) {
		for _, busy := range []bool{false, true} {
			m := fixture()
			m.width, m.height, m.section, m.focus = 120, 36, 1, 0
			m.data.Busy = busy
			m.perform("select")
			if !m.branchSelection[m.currentID()] {
				t.Errorf("busy=%v selection rejected: %s", busy, m.status)
			}
		}
	})
	t.Run("auto_grid_page", func(t *testing.T) {
		m := simulatorFixture()
		m.gridPinned = false
		for i := 2; i < 6; i++ {
			m.simulation.Conversations = append(m.simulation.Conversations, simulationConversation{Index: i, Status: "complete"})
		}
		r := rect{0, 0, 90, 26}
		idle := m.gridPage(r)
		m.simulation.Conversations[4].Status = "running"
		if idle != m.gridPage(r) {
			t.Fatal("background generation moved grid page")
		}
	})
	t.Run("row_identity_on_insert", func(t *testing.T) {
		m := fixture()
		m.width, m.height, m.section, m.focus = 120, 36, 1, 0
		root := m.data.Nodes[0]
		m.data.Nodes = append(m.data.Nodes, node{ID: "reading", Title: "Reading", Text: "reading"})
		m.selected = 1
		before := m.targetRow().id
		next := m.data
		next.Busy = true
		next.Nodes = append(next.Nodes, node{ID: "child", Parent: root.ID, Text: "generating"})
		raw, _ := json.Marshal(next)
		m.apply(event{Type: "state", Data: raw})
		if before != m.targetRow().id {
			t.Fatalf("background insertion row %s -> %s", before, m.targetRow().id)
		}
	})
}

func TestPendingOpenIsCanceledByNavigationOrError(t *testing.T) {
	for _, failure := range []bool{false, true} {
		m := simulatorFixture()
		m.focus, m.selected = 0, 3
		m.previewTarget()
		request := m.previewRequest
		m.activate()
		if failure {
			m.apply(event{Type: "error", ID: request, Data: json.RawMessage(`{"message":"unavailable"}`)})
		} else {
			m.selected = 4
			m.previewTarget()
			run := *m.simulation
			i := 0
			run.Browsed = true
			run.OpenConversation = &i
			raw, _ := json.Marshal(run)
			m.apply(event{Type: "simulation", ID: request, Data: raw})
		}
		if m.pendingActivation != nil || m.simSelection != nil || m.focus != 0 {
			t.Fatal("stale activation survived")
		}
	}
}
func TestGridFollowsOnlyExplicitActiveAction(t *testing.T) {
	m := simulatorFixture()
	m.data.Busy = true
	for i := 2; i < 6; i++ {
		m.simulation.Conversations = append(m.simulation.Conversations, simulationConversation{Index: i, Status: "complete"})
	}
	m.activeSimulation = m.simulation
	m.simulation.Conversations[4].Status = "running"
	m.openActive()
	if !m.gridFollow || m.gridPage(rect{0, 0, 90, 26}) != 1 {
		t.Fatal("explicit follow did not follow live output")
	}
	m.gridKey("nav.left")
	if m.gridFollow {
		t.Fatal("manual navigation did not stop follow")
	}
}

func TestOpeningPreviewGridRemembersTile(t *testing.T) {
	m := simulatorFixture()
	m.gridPinned = false
	m.openGridTile(1)
	m.simulatorBack()
	if m.gridSelected(rect{0, 0, 90, 26}) != 1 {
		t.Fatal("back lost opened tile")
	}
}

func TestLiveFollowKeepsLastPageOnCompletion(t *testing.T) {
	m := simulatorFixture()
	m.data.Busy = true
	for i := 2; i < 6; i++ {
		m.simulation.Conversations = append(m.simulation.Conversations, simulationConversation{Index: i, Status: "complete"})
	}
	m.gridPinned = false
	m.gridFollow = true
	m.simulation.Conversations[4].Status = "running"
	m.apply(event{Type: "simulation.token", Data: json.RawMessage(`{"run":"batch","conversation":4,"turn":0,"text":"x"}`)})
	m.simulation.Conversations[4].Status = "complete"
	next := m.data
	next.Busy = false
	raw, _ := json.Marshal(next)
	m.apply(event{Type: "state", Data: raw})
	if m.gridFollow || m.gridPage(rect{0, 0, 90, 26}) != 1 {
		t.Fatal("completion moved last live page")
	}
}
