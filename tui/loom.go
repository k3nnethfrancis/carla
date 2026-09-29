package main

import tea "charm.land/bubbletea/v2"

// The view determines document versus conversation generation; an explicit
// Simulator selection determines which histories advance.
func (m *model) loom(options generationOptions) tea.Cmd {
	if options.Action == "" {
		options.Action = "loom"
	}
	if m.section != 3 && (options.Turns > 0 || options.Message != "" || options.VisitorModel != "") {
		m.status = "Error: --turns, --visitor and --visitor-model apply only to Simulator conversations"
		return nil
	}
	if m.pending || m.data.Busy {
		m.status = "Wait for the current operation, or /stop it"
		return nil
	}
	if m.editing != "" {
		m.status = "Save or cancel the edit before running /loom"
		return nil
	}
	if m.section == 3 {
		args, _, err := m.simulationLoomPlan(options)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		if options.Action == "continue" && m.simulation == nil && m.simSelection != nil && m.simSelection.Group == "" {
			m.awaitingSimulation = true
			m.preserveSimulationSelection = true
		}
		if options.Action == "loom" {
			m.preserveSimulationSelection = false
			m.awaitingSimulation = true
			m.conversationOpen = false
			m.simulation = nil
			m.activeSimulation = nil
		}
		return m.send("simulator.run", args)
	}
	if options.Count == 0 && options.Action == "loom" {
		options.Count = 1
	}
	if options.Loops == 0 && options.Action == "loom" {
		options.Loops = 1
	}

	if m.section == 0 {
		refs := m.data.Selected
		if len(refs) == 0 {
			refs = m.targetRefs()
		}
		args := map[string]any{"refs": refs}
		options.apply(args)
		m.section, m.focus = 1, 1
		return m.sendDocumentGeneration(args)
	}
	if m.selectionVisible() || m.targetRow().kind == "document-set" {
		args := m.documentGroupArgs()
		options.apply(args)
		m.section, m.focus = 1, 1
		return m.sendDocumentGeneration(args)
	}
	if m.targetRow().kind != "node" {
		m.status = "Select a document in Branches to run /loom"
		return nil
	}
	if m.targetRow().id != m.currentID() || (m.commandDocument != "" && m.commandDocument != m.currentID()) {
		m.status = "Wait for the selected document to load before running /loom"
		return m.previewTarget()
	}
	m.section, m.focus = 1, 1
	m.reflow()
	return m.generateAtCursor(true, options)
}
