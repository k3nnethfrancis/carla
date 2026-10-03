package main

import (
	"strings"
	"testing"
)

func TestSelectionResultsUseLatestAttemptAndKeepGenerationStatus(t *testing.T) {
	m := policyFixture()
	m.data.PolicyRuns = []policyRun{
		{ID: "original", Status: "failed", Loops: 2, Steps: []policyStep{{Loop: 1, Outcomes: map[string]string{"0": "assessment error"}}}},
		{ID: "retry", RetryOf: "original", Status: "complete", Loops: 2, Steps: []policyStep{{Loop: 1, Outcomes: map[string]string{"0": "chosen"}}}},
	}
	m.data.SimulationRuns = []runSummary{{ID: "sim", PolicyRun: "original", Loop: 1, AlternativeIndex: 0}}
	m.simulation = &simulationRun{ID: "sim", Conversations: []simulationConversation{{Index: 0, Status: "complete"}}}
	m.section = 3
	items := m.gridItems()
	if len(items) != 1 || items[0].Status != "complete · chosen" || !strings.Contains(items[0].Title, "L1 · B1") {
		t.Fatal(items)
	}
	m.openSelectionResults()
	if m.dialog.args["run"] != "retry" {
		t.Fatal("opened old attempt")
	}
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	if m.dialog.rows[0].id != "cancel" {
		t.Fatal("retry must require confirmation")
	}
	m.submitDialog()
	if m.dialog.kind != "selection-results" {
		t.Fatal("cancel lost results")
	}
}
