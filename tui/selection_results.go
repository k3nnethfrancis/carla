package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"sort"
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
func (m *model) openSelectionResults() tea.Cmd {
	r := m.selectionRunSummary(m.simulation.ID)
	p := m.selectionAttempt(r.PolicyRun)
	if p.ID == "" {
		return m.send("simulator.inspect", map[string]any{"run": m.simulation.ID})
	}
	m.openSelectionAttempt(p)
	m.dialog.args["simulation"] = m.simulation.ID
	return nil
}

func (m *model) openSelectionAttempt(p policyRun) tea.Cmd {
	d := &dialog{kind: "selection-results", title: fmt.Sprintf("Selection · %s", p.Status), args: map[string]any{"run": p.ID}}
	for _, step := range p.Steps {
		keys := make([]string, 0, len(step.Outcomes))
		for key := range step.Outcomes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			label := key
			if i, err := strconv.Atoi(key); err == nil {
				label = fmt.Sprintf("branch-%d", i+1)
			}
			d.rows = append(d.rows, row{id: "inspect", label: fmt.Sprintf("Loop %d · %s · %s", step.Loop, label, step.Outcomes[key]), preview: step.Reasons[key]})
		}
	}
	d.rows = append(d.rows, row{id: "inspect", label: "Exact judge traces", preview: "Saved inputs, responses, evidence and errors for this attempt."})
	d.rows = append(d.rows, row{id: "generation", label: "Generation trace", preview: "Original model requests and saved conversation or document."})
	d.rows = append(d.rows, row{id: "retry", label: "Retry selection", preview: "Reassess the last loop's saved candidates with its original policy. Saves a new attempt; generates no conversations and starts no further loops."})
	m.dialog = d
	return nil
}
func (m *model) submitSelectionResults() tea.Cmd {
	d := m.dialog
	id := d.rows[d.index].id
	if id == "cancel" {
		return m.closeDialog()
	}
	if id == "retry" {
		m.dialog = &dialog{kind: "selection-results", title: "Retry saved selection?", parent: d, args: d.args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "confirm", label: "Retry assessment and choice", preview: "Uses the saved policy and candidate text. New results are retained separately. No generation or automatic next loop."}}}
		return nil
	}
	m.dialog = nil
	if id == "generation" {
		if run, ok := d.args["simulation"]; ok {
			return m.send("simulator.inspect", map[string]any{"run": run})
		}
		return m.send("inspect", nil)
	}
	if id == "confirm" {
		return m.send("policy.retry", map[string]any{"run": d.args["run"]})
	}
	return m.send("inspect", map[string]any{"run": d.args["run"]})
}

func (m *model) documentSelectionAttempt() policyRun {
	for i := len(m.data.PolicyRuns) - 1; i >= 0; i-- {
		p := m.data.PolicyRuns[i]
		for _, step := range p.Steps {
			for _, id := range step.Candidates {
				if id == m.currentID() {
					return m.selectionAttempt(p.ID)
				}
			}
		}
	}
	return policyRun{}
}
