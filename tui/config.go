package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
)

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
	if m.selectionEnabled() {
		label = "Selection · On"
	}
	d := &dialog{kind: "grow-config", title: "Selection policy", rows: []row{
		{id: "selection_enabled", label: label, preview: selectionTriggerHelp},
		{id: "judges", label: "Judges · 1", preview: "The selection model and the behaviors it uses to judge alternatives. Selection currently uses one LLM judge."},
	}}
	defer orderOperationalRows(d)
	m.appendOperationalControls(d)
	m.dialog = d
	return nil
}

// Keep the trigger first so compact dialogs show it before any clipped detail.
const selectionTriggerHelp = "On + 2+ alternatives + explicit --loops.\n/loom 4 --selection on --loops 1\nOne loop selects; more split from the winner.\nNone qualify: stop. Other outputs stay saved."
const evaluationJudgesHelp = "Criteria + model used by named evaluations.\nChoose judges in Evaluate > Configure.\nRun: /eval or /loom --eval \"name\"\nCreating a judge does not run it."

func (m *model) openSelectionJudges() tea.Cmd {
	m.dialog = &dialog{kind: "grow-config", title: "Selection judges", rows: []row{
		{id: "judge", label: m.selectionModelName() + " (LLM)", preview: "Compares candidates together and selects which qualifying candidate continues."},
	}}
	return nil
}
func (m *model) openSelectionJudge() tea.Cmd {
	m.dialog = &dialog{kind: "grow-config", title: "Selection judge", rows: []row{
		{id: "selector", label: "Model · " + m.selectionModelName() + " (LLM)", preview: "The local instruction-following model used to assess alternatives."},
		{id: "prompt", label: "Prompt", preview: "Judges candidates together in one request. The prompt defines the required JSON selection response; separate behavior calls are not used."},
		{id: "behaviors", label: fmt.Sprintf("Behaviors · %d", len(m.selectionBehaviors())), preview: "Selection criteria define which candidates qualify and are worth continuing."},
	}}
	return nil
}
func (m *model) openSelectionBehaviors() tea.Cmd {
	d := &dialog{kind: "grow-config", title: "Selection behaviors"}
	for _, b := range m.selectionBehaviors() {
		id, _ := b["id"].(string)
		name, _ := b["name"].(string)
		spec, _ := b["spec"].(string)
		d.rows = append(d.rows, row{id: id, label: name, preview: "Criteria applied to candidates. " + spec})
	}
	d.rows = append(d.rows, row{id: "new-behavior", label: "+ New behavior", preview: "Add another named criterion for this judge."}, row{id: "library-behavior", label: "From library", preview: "Copy a saved behavior definition into this judge."})
	m.dialog = d
	return nil
}
func (m *model) openSelectionBehavior() tea.Cmd {
	id := m.operationalBehaviorID()
	if id == "" {
		id = "criteria"
	}
	name, spec, enabled := "Selection criteria", "", false
	for _, b := range m.selectionBehaviors() {
		if b["id"] == id {
			name, _ = b["name"].(string)
			spec, _ = b["spec"].(string)
			enabled, _ = b["enabled"].(bool)
		}
	}
	state := "Off"
	if enabled {
		state = "On"
	}
	m.dialog = &dialog{kind: "grow-config", title: "Selection behavior", args: map[string]any{"selection_behavior": id}, rows: []row{
		{id: "behavior-enabled", label: "Enabled · " + state, preview: "Include this criterion in the selection judge’s candidate assessment."},
		{id: "spec", label: "Behavior spec · " + name, preview: "Defines which candidates qualify. " + spec},
	}}
	return nil
}
