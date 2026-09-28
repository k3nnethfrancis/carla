package main

import tea "charm.land/bubbletea/v2"

// Configuration is a contextual view, not a separate settings copy per tab.
func (m *model) openConfig() tea.Cmd {
	if m.section == 3 {
		return m.openSimulatorConfig()
	}
	m.dialog = &dialog{kind: "loom-config", title: "Document Loom", rows: []row{
		{id: "models", label: "Generation model"},
		{id: "settings", label: "Sampling and token ceiling", preview: "Shared by Library, Branches and Anthology. Bare /loom creates one continuation."},
		{id: "selection", label: "Selection policy", preview: "Classify alternatives and advance one eligible result between loops."},
		{id: "monitor", label: "Monitoring policy", preview: "Shared monitoring specs; warn or explicitly stop. Independent of selection."},
	}}
	return nil
}
func (m *model) openSelectionConfig() tea.Cmd {
	m.dialog = &dialog{kind: "grow-config", title: "Selection policy", rows: []row{
		{id: "selector", label: "Evaluator model", preview: m.data.PolicyModel},
		{id: "spec", label: "Selection criteria", preview: m.data.PolicySpec},
		{id: "prompt", label: "Classifier and routing prompt", preview: m.data.PolicyPrompt},
	}}
	return nil
}
