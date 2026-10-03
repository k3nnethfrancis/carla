package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strconv"
	"strings"
)

// Tree labels stay local: the visible ancestors already explain their lineage.
// Headings retain the full name; a user title always takes precedence.
func simulationName(title, local, full, fallback string) string {
	for _, name := range []string{title, local, full, fallback} {
		if name != "" {
			return name
		}
	}
	return ""
}
func conversationName(c simulationConversation, compact bool) string {
	local := ""
	if compact {
		local = c.ShortLabel
	}
	return simulationName(c.Title, local, c.Label, fmt.Sprintf("convo-%d", c.Index+1))
}
func conversationRowLabel(c simulationConversation, compact bool) string {
	return fmt.Sprintf("%s · %d messages · %s", conversationName(c, compact), max(c.TurnCount, len(c.Turns)), conversationStatus(c))
}
func (m *model) renameSimulation() tea.Cmd {
	if m.pending {
		return nil
	}
	var args map[string]any
	title := ""
	if target, ok := m.conversationTarget(); ok {
		args = target
		for _, run := range m.simulationSummaries() {
			if run.ID == target["run"] {
				for _, c := range run.Conversations {
					if c.Index == target["conversation"] {
						title = c.Title
					}
				}
			}
		}
	} else {
		r := m.targetRow()
		switch r.kind {
		case "simulation":
			args = map[string]any{"run": r.id}
			for _, run := range m.simulationSummaries() {
				if run.ID == r.id {
					title = run.Title
				}
			}
		case "simulation-group":
			parts := strings.SplitN(r.id, "/", 2)
			args = map[string]any{"group": parts[0]}
			if len(parts) == 2 {
				i, err := strconv.Atoi(parts[1])
				if err != nil {
					args = map[string]any{"scope": r.id}
					if scope := m.simulationGroupScope(r.id); scope != nil {
						title = scope.Title
					}
				}
				if err == nil {
					args["alternative"] = i
				}
			}
			for _, run := range m.simulationSummaries() {
				if run.AlternativeGroup == parts[0] {
					if a, ok := args["alternative"]; ok {
						if run.AlternativeIndex == a {
							title = run.AlternativeTitle
						}
					} else {
						title = run.OperationTitle
					}
				}
			}
		default:
			m.status = "Focus a conversation or Loom to rename"
			return nil
		}
	}
	d := &dialog{kind: "sim-rename", title: "Rename conversation or Loom", args: args}
	d.add("Name (blank restores automatic)", title)
	d.add("Update child ancestry names (SPACE toggles)", "Off")
	m.dialog = d
	return d.fields[0].input.Focus()
}

// A live payload can carry a rename before the next snapshot refresh.
func copySimulationNames(summary *runSummary, run *simulationRun) {
	summary.Label, summary.ShortLabel, summary.Title = run.Label, run.ShortLabel, run.Title
	summary.OperationLabel, summary.OperationShortLabel, summary.OperationTitle = run.OperationLabel, run.OperationShortLabel, run.OperationTitle
	summary.AlternativeLabel, summary.AlternativeShortLabel, summary.AlternativeTitle = run.AlternativeLabel, run.AlternativeShortLabel, run.AlternativeTitle
}
