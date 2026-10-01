package main

import tea "charm.land/bubbletea/v2"

// Configuration is a contextual view, not a separate settings copy per tab.
func (m *model) openConfig() tea.Cmd {
	defer func() {
		if m.dialog != nil {
			m.dialog.rows = append(m.dialog.rows, row{id: "workspaces", label: "Workspace"}, row{id: "keys", label: "Keybindings"})
		}
	}()
	if m.section == 4 {
		if m.evalArea == "policies" {
			return m.openEvaluationPolicies()
		}
		cmd := m.openCollectionConfig()
		if m.dialog == nil {
			m.dialog = &dialog{kind: "loom-config", title: "Configuration"}
		}
		return cmd
	}
	if m.section == 3 {
		return m.openSimulatorConfig()
	}
	m.dialog = &dialog{kind: "loom-config", title: "Document Loom", rows: []row{
		{id: "models", label: "Generation model"},
		{id: "settings", label: "Sampling and token ceiling", preview: "Document generation settings. Bare /loom continues once; a count of two or more splits alternatives."},
	}}
	return nil
}
func (m *model) openSelectionConfig() tea.Cmd {
	label := "Selection · Off"
	if m.data.SelectionEnabled {
		label = "Selection · On"
	}
	m.dialog = &dialog{kind: "grow-config", title: "Selection policy", rows: []row{
		{id: "selection_enabled", label: label, preview: selectionTriggerHelp},
		{id: "selector", label: "Selection model", preview: "Local instruct model that judges the alternatives.\nCurrent: " + m.data.PolicyModel},
		{id: "spec", label: "Selection criteria", preview: "What makes a candidate worth continuing.\n" + m.data.PolicySpec},
		{id: "prompt", label: "Selection prompt", preview: "Instructions and required response format for the selector.\n" + m.data.PolicyPrompt},
	}}
	return nil
}

// Keep the trigger first so compact dialogs show it before any clipped detail.
const selectionTriggerHelp = "On + 2+ alternatives + explicit --loops.\n/loom 4 --selection on --loops 1\nOne loop selects; more split from the winner.\nNone qualify: stop. Other outputs stay saved."
const evaluationJudgesHelp = "Criteria + model used by named evaluations.\nChoose judges in Evaluate > Configure.\nRun: /eval or /loom --eval \"name\"\nCreating a judge does not run it."
