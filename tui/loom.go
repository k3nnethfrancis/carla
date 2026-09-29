package main

import tea "charm.land/bubbletea/v2"

// The view determines document versus conversation generation; an explicit
// Simulator selection determines which histories advance.
func (m *model) loom(options generationOptions) tea.Cmd {
	if m.section != 3 && (options.Turns > 0 || options.Message != "") {
		m.status = "Error: --turns and --message apply only to Simulator conversations"
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
		m.conversationOpen = false
		m.simulation = nil
		m.activeSimulation = nil
		return m.send("simulator.run", args)
	}
	if options.Count == 0 {
		options.Count = 1
	}
	if options.Loops == 0 {
		options.Loops = 1
	}

	if m.section == 0 {
		args := map[string]any{"refs": m.targetRefs()}
		options.apply(args)
		m.section, m.focus = 1, 1
		return m.send("continue", args)
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
