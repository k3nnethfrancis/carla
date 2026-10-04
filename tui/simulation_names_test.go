package main

import (
	"strings"
	"testing"
)

func TestSimulationNamesCompactTreeFullHeadingAndRename(t *testing.T) {
	m := simulatorFixture()
	m.simulation.Label = "loom-1"
	m.simulation.ShortLabel = "loom-1"
	for i := range m.simulation.Conversations {
		c := &m.simulation.Conversations[i]
		c.Label = "convo-" + string(rune('1'+i)) + "-loom-1"
		c.ShortLabel = "convo-" + string(rune('1'+i))
	}
	rows := m.rows()
	if !strings.Contains(rows[2].label, "loom-1") || strings.Contains(rows[3].label, "convo-1-loom-1") || !strings.Contains(rows[3].label, "convo-1") {
		t.Fatalf("compact labels: %#v", rows)
	}
	m.openGridTile(1)
	if !strings.HasPrefix(m.conversationHeading(80), "convo-2-loom-1") {
		t.Fatal(m.conversationHeading(80))
	}
	m.renameSimulation()
	if m.dialog == nil || m.dialog.kind != "sim-rename" || m.dialog.args["conversation"] != 1 {
		t.Fatal("rename did not target opened conversation")
	}
	m.simulation.Conversations[1].Title = "the patient gardener"
	if !strings.HasPrefix(m.conversationHeading(80), "the patient gardener") {
		t.Fatal("custom name missing")
	}
}

func TestSimulationRenameGroupAndScopeNames(t *testing.T) {
	m := simulatorFixture()
	m.data.SimulationRuns = []runSummary{{ID: "a", AlternativeGroup: "group", AlternativeCount: 2, AlternativeIndex: 0, OperationLabel: "loom-2-loom-1", OperationShortLabel: "loom-2", Conversations: []simulationConversation{{Index: 0, Label: "branch-1-convo-1-loom-1", ShortLabel: "branch-1"}}}}
	m.simulation = nil
	rows := m.rows()
	for i, r := range rows {
		if strings.Contains(r.label, "Alternative") {
			t.Fatal(r.label)
		}
		if r.id == "group/0" {
			m.selected = i
			m.renameSimulation()
			if m.dialog == nil || m.dialog.args["group"] != "group" || m.dialog.args["alternative"] != 0 {
				t.Fatal("rename subgroup target")
			}
		}
	}
}

func TestStandaloneConversationRowsKeepDistinctFullNames(t *testing.T) {
	m := simulatorFixture()
	m.simulation = nil
	m.data.SimulationRuns = []runSummary{
		{ID: "one", Conversations: []simulationConversation{{Index: 0, Label: "convo-1-loom-1", ShortLabel: "convo-1"}}},
		{ID: "two", Conversations: []simulationConversation{{Index: 0, Label: "convo-1-loom-2", ShortLabel: "convo-1"}}},
	}
	rows := m.rows()
	if !strings.Contains(rows[2].label, "convo-1-loom-1") || !strings.Contains(rows[3].label, "convo-1-loom-2") {
		t.Fatalf("%#v", rows)
	}
}

// A nested scope is an exact target, never a parse failure that renames its Loom.
func TestRenameNestedScopeDoesNotRenameContainingLoom(t *testing.T) {
	m := simulatorFixture()
	m.simulation = nil
	tree := actionScope{Kind: "set", ID: "g/0/s", Children: []actionScope{
		{Kind: "set", ID: "g/0/s.0", Title: "inner", Children: []actionScope{{Kind: "conversation", Run: "a", Conversation: 0}, {Kind: "conversation", Run: "b", Conversation: 0}}},
	}}
	m.data.SimulationRuns = []runSummary{
		{ID: "a", AlternativeGroup: "g", AlternativeCount: 1, AlternativeScope: &tree, Conversations: []simulationConversation{{Index: 0}}},
		{ID: "b", AlternativeGroup: "g", AlternativeCount: 1, AlternativeScope: &tree, Conversations: []simulationConversation{{Index: 0}}},
	}
	found := false
	for i, r := range m.rows() {
		if r.id == "g/0/s.0" {
			found = true
			m.selected = i
			m.focus = 0
			m.renameSimulation()
			if m.dialog == nil || m.dialog.args["scope"] != r.id || m.dialog.args["group"] != nil {
				t.Fatal("wrong nested rename target")
			}
		}
	}
	if !found {
		t.Fatal("nested scope missing")
	}
}
