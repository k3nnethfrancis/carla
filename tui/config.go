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
		{id: "selection_enabled", label: label, preview: "Choose whole alternatives between split loops. Off continues every alternative; one-output loops always continue directly."},
		{id: "selector", label: "Evaluator model", preview: m.data.PolicyModel},
		{id: "spec", label: "Selection criteria", preview: m.data.PolicySpec},
		{id: "prompt", label: "Classifier and routing prompt", preview: m.data.PolicyPrompt},
	}}
	return nil
}
