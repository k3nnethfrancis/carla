package main

import tea "charm.land/bubbletea/v2"

// One command, with its target fixed by the current tab rather than a stale
// document selection. Simulator runs start fresh from its configured opening.
func (m *model) loom(options generationOptions) tea.Cmd {
	if m.section != 1 && m.section != 3 {
		m.status = "Open the Branches or Simulator tab to run /loom"
		return nil
	}
	if m.section == 1 && options.Turns > 0 {
		m.status = "Error: --turns applies only to Simulator conversations"
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
		args := map[string]any{}
		options.apply(args)
		if target, ok := m.conversationTarget(); ok {
			for k, v := range target {
				args[k] = v
			}
		}
		m.conversationOpen = false
		m.simulation = nil
		m.activeSimulation = nil
		return m.send("simulator.run", args)
	}
	if m.targetRow().kind != "node" {
		m.status = "Select a document in Branches to run /loom"
		return nil
	}
	if m.targetRow().id != m.currentID() || (m.commandDocument != "" && m.commandDocument != m.currentID()) {
		m.status = "Wait for the selected document to load before running /loom"
		return m.previewTarget()
	}
	m.focus = 1
	m.reflow()
	return m.generateAtCursor(true, options)
}
