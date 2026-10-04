package main

import (
	tea "charm.land/bubbletea/v2"
	"strconv"
)

func (m *model) selectionAttempt(id string) policyRun {
	var result policyRun
	known := map[string]bool{id: true}
	for _, p := range m.data.PolicyRuns {
		if known[p.ID] || known[p.RetryOf] {
			result = p
			known[p.ID] = true
		}
	}
	return result
}
func (m *model) selectionOutcome(r runSummary) string {
	p := m.selectionAttempt(r.PolicyRun)
	for _, step := range p.Steps {
		if step.Loop == r.Loop {
			return step.Outcomes[strconv.Itoa(r.AlternativeIndex)]
		}
	}
	return ""
}
func (m *model) selectionRunSummary(id string) runSummary {
	for _, r := range m.simulationSummaries() {
		if r.ID == id {
			return r
		}
	}
	return runSummary{}
}

// Retry is the only mutating action reachable from inspection and is confirmed.
func (m *model) submitSelectionResults() tea.Cmd {
	d := m.dialog
	if d.rows[d.index].id == "cancel" {
		return m.closeDialog()
	}
	m.dialog = nil
	m.focus = m.inspectionOrigin
	m.inspectionParent = nil
	m.showInspector = false
	m.reflow()
	return m.send("policy.retry", map[string]any{"run": d.args["run"]})
}
