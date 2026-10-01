package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strconv"
)

func (m *model) simString(key string) string { v, _ := m.data.SimulatorConfig[key].(string); return v }
func (m *model) simDocs() []string {
	out := []string{}
	if values, ok := m.data.SimulatorConfig["documents"].([]any); ok {
		for _, v := range values {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
func (m *model) simulationText() string {
	if m.simulation == nil && m.simSelection != nil && m.simSelection.Group != "" {
		return fmt.Sprintf("%d conversations selected across alternative sets.\n\nOpen a child set to compare its conversations.\n\n/continue advances this selection.\n/loom N creates N alternative futures of this selection.", len(m.selectedConversations()))
	}
	if m.simulation == nil {
		return fmt.Sprintf("Choose anthology documents and configure a run.\n\nDocuments: %d\nCharacter: %s\nVisitor: %s\n\nOpening: %s\n\nThe character receives the selected documents. The visitor receives its own brief and conversation history. Both use raw local completions.\n\nUse /config, then /loom. /inspect shows a selected run’s exact requests.",
			len(m.simDocs()), m.simString("character_alias"), m.simString("visitor_alias"), m.openingDescription())
	}
	if m.conversationOpen {
		return m.conversationDocument(max(1, m.document.Width()))
	}
	return fmt.Sprintf("%s · %s\n\nSelect a conversation, or open the Loom grid.", simulationName(m.simulation.Title, "", m.simulation.Label, m.simulation.ID), m.simulation.Status)
}

func (m *model) openSimulatorConfig() tea.Cmd {
	d := &dialog{kind: "sim-config", title: "Simulator"}
	for _, entry := range []struct{ key, label string }{
		{"documents", "Anthology documents"}, {"character_alias", "Character model"}, {"visitor_alias", "Visitor model"},
		{"openings", "Opening"}, {"turns", "Turns per continuation"}, {"visitor_brief", "Visitor brief"},
		{"character_settings", "Character sampling"}, {"visitor_settings", "Visitor sampling"},
		{"character_template", "Character prompt"}, {"visitor_template", "Visitor prompt"},
	} {
		value := fmt.Sprint(m.data.SimulatorConfig[entry.key])
		if entry.key == "documents" {
			value = fmt.Sprintf("%d explicitly selected versions", len(m.simDocs()))
		}
		label := entry.label
		switch entry.key {
		case "character_alias", "visitor_alias":
			label += " · " + m.modelName(m.simString(entry.key))
		case "openings":
			label += " · " + m.simString("opening_mode")
			value = "Fixed message or a freshly generated opening for each conversation"
		case "documents":
			label += fmt.Sprintf(" · %d selected", len(m.simDocs()))
		case "conversations", "turns":
			label += " · " + value
		}
		d.rows = append(d.rows, row{id: entry.key, label: label, preview: value})
	}
	m.dialog = d
	return nil
}
func (m *model) openGrowConfig() tea.Cmd {
	s := m.data.GrowSettings
	m.dialog = &dialog{kind: "grow-config", title: "Grow", rows: []row{
		{id: "models", label: "Generator", preview: m.data.ModelAlias + " · current branch generator"},
		{id: "selector", label: "Selector model", preview: m.data.PolicyModel},
		{id: "spec", label: "Selection criteria", preview: m.data.PolicySpec},
		{id: "prompt", label: "Selector prompt", preview: m.data.PolicyPrompt},
		{id: "count", label: fmt.Sprintf("Alternatives: %d", s.Count), preview: "Candidates sampled and compared each cycle."},
		{id: "rounds", label: fmt.Sprintf("Cycles: %d", s.Rounds), preview: "Generate → select → continue. Stops early when nothing is selected."},
		{id: "n_predict", label: fmt.Sprintf("Output budget: %s", budgetLabel(s.Tokens)), preview: "Per candidate; independent of Simulator."},
		{id: "temperature", label: fmt.Sprintf("Temperature: %g", s.Temperature)},
		{id: "top_p", label: fmt.Sprintf("Top-p: %g", s.TopP)},
	}}
	return nil
}
func budgetLabel(n int) string {
	if n == -1 {
		return "Max"
	}
	return strconv.Itoa(n)
}
func (m *model) configureChoice(d *dialog, r row) tea.Cmd {
	// Preserve the actual parent page, including its selected row and ancestry.
	defer func() {
		if m.dialog != nil && m.dialog != d.parent && m.dialog != d {
			m.dialog.parent = d
		}
		if m.editing != "" {
			m.editReturn = d
		}
	}()
	switch d.kind {
	case "loom-config":
		switch r.id {
		case "selection":
			return m.openOperationalPolicies("selection")
		case "monitor":
			return m.openOperationalPolicies("monitoring")
		default:
			return m.perform(r.id)
		}
	case "grow-config":
		m.dialog = d
		if cmd, ok := m.operationalControl(d, r.id); ok {
			return cmd
		}
		if d.title == "Selection behaviors" && r.id != "new-behavior" && r.id != "library-behavior" {
			d.args = map[string]any{"selection_behavior": r.id}
			m.openSelectionBehavior()
			m.dialog.parent = d
			return nil
		}
		switch r.id {
		case "new-behavior":
			m.dialog = &dialog{kind: "operational-policy-behavior-name", title: "New behavior", parent: d, args: map[string]any{}}
			m.dialog.add("Name", "")
			return m.dialog.fields[0].input.Focus()
		case "library-behavior":
			m.openBehaviorLibraryPicker(d, "operational-policy-library-selection", nil)
			return nil
		case "behavior-enabled":
			id := m.operationalBehaviorID()
			enabled := false
			for _, b := range m.selectionBehaviors() {
				if b["id"] == id {
					enabled, _ = b["enabled"].(bool)
				}
			}
			return m.saveSelectionBehavior(id, map[string]any{"enabled": !enabled})

		case "judges":
			m.openSelectionJudges()
			m.dialog.parent = d
			return nil
		case "judge":
			m.openSelectionJudge()
			m.dialog.parent = d
			return nil
		case "behaviors":
			m.openSelectionBehaviors()
			m.dialog.parent = d
			return nil
		case "behavior":
			m.openSelectionBehavior()
			m.dialog.parent = d
			return nil
		case "selection_enabled":
			return m.send("policy.configure", map[string]any{"selection_enabled": !m.selectionEnabled()})
		case "models":
			return m.openDialog("models")
		case "spec":
			return m.beginEdit("policy_spec")
		case "prompt":
			return m.beginEdit("policy_prompt")
		case "selector":
			next := &dialog{kind: "selector-pick", title: "Grow selector"}
			for _, model := range m.data.SelectorModels {
				next.rows = append(next.rows, row{id: model.Alias, label: model.Name, preview: "Local selector; selection criteria stay out of generator context."})
			}
			next.rows = append(next.rows, row{id: "setup", label: "+ Add model"})
			m.dialog = next
			return nil
		}
		values := map[string]float64{"count": float64(m.data.GrowSettings.Count), "rounds": float64(m.data.GrowSettings.Rounds), "n_predict": float64(m.data.GrowSettings.Tokens), "temperature": m.data.GrowSettings.Temperature, "top_p": m.data.GrowSettings.TopP}
		return m.numberConfig("grow", r.id, values[r.id], "")
	case "selector-pick":
		return m.saveDialog(d, "grow.selector", map[string]any{"alias": r.id})
	case "sim-config", "sim-speakers", "sim-openings":
		switch r.id {
		case "selection":
			return m.openOperationalPolicies("selection")
		case "monitor":
			return m.openOperationalPolicies("monitoring")
		case "openings":
			return m.openOpeningConfig()
		case "opening_mode":
			m.dialog = &dialog{kind: "sim-opening-mode", title: "Opening mode", rows: []row{
				{id: "fixed", label: "Fixed", preview: "Same opening message for every conversation"},
				{id: "generated", label: "Generated", preview: "Fresh opening per conversation; no anthology supplied"},
			}}
		case "preview_openings":
			m.simulation = nil
			m.dialog = nil
			return m.send("simulator.preview", nil)
		case "documents":
			selected := map[string]bool{}
			for _, id := range m.simDocs() {
				selected[id] = true
			}
			next := &dialog{kind: "sim-documents", title: "Anthology · SPACE select · CTRL+S save", args: map[string]any{"selected": selected}}
			for _, n := range m.data.Nodes {
				if n.Kept {
					label := documentLabel(n)
					if selected[n.ID] {
						label = "✓ " + label
					}
					next.rows = append(next.rows, row{id: n.ID, label: label, preview: n.Preview})
				}
			}
			m.dialog = next
		case "character_alias", "visitor_alias", "opening_alias":
			next := &dialog{kind: "sim-model", title: r.label, args: map[string]any{"field": r.id}}
			if r.id == "opening_alias" {
				next.rows = append(next.rows, row{id: "", label: "Use visitor model", preview: m.modelName(m.simString("visitor_alias"))})
			}
			for _, model := range m.data.Models {
				next.rows = append(next.rows, row{id: model.Alias, label: model.Name})
			}
			next.rows = append(next.rows, row{id: "setup", label: "+ Add model"})
			m.dialog = next
		case "character_template", "visitor_template", "visitor_brief", "opening_prompt":
			return m.beginEdit(r.id)
		case "opening", "monitor_model":
			next := &dialog{kind: "sim-text", title: r.label, args: map[string]any{"field": r.id}}
			next.add(r.label, m.simString(r.id))
			m.dialog = next
			return next.fields[0].input.Focus()
		case "character_settings", "visitor_settings", "opening_settings":
			next := &dialog{kind: "sim-sampling", title: r.label, args: map[string]any{"group": r.id}}
			values, _ := m.data.SimulatorConfig[r.id].(map[string]any)
			for _, key := range []string{"n_predict", "temperature", "top_p"} {
				next.rows = append(next.rows, row{id: key, label: samplingLabel(key) + " · " + samplingValue(key, values[key])})
			}
			m.dialog = next
		default:
			value, _ := m.data.SimulatorConfig[r.id].(float64)
			return m.numberConfig("sim", r.id, value, "")
		}
	case "sim-opening-mode":
		return m.saveDialog(d, "simulator.configure", map[string]any{"opening_mode": r.id})
	case "sim-model":
		return m.saveDialog(d, "simulator.configure", map[string]any{d.args["field"].(string): r.id})
	case "sim-sampling":
		group := d.args["group"].(string)
		values, _ := m.data.SimulatorConfig[group].(map[string]any)
		value, _ := values[r.id].(float64)
		return m.numberConfig("sim", r.id, value, group)
	}
	return nil
}
func (m *model) numberConfig(scope, key string, value float64, group string) tea.Cmd {
	d := &dialog{kind: "config-number", title: samplingLabel(key) + " · ↑↓ adjust · ←→ ×10", args: map[string]any{"scope": scope, "key": key, "group": group}}
	label := "Value"
	if key == "n_predict" {
		label = "Tokens · M = Max"
	}
	display := strconv.FormatFloat(value, 'f', -1, 64)
	if key == "n_predict" && value == -1 {
		display = "Max"
	}
	d.add(label, display)
	m.dialog = d
	return nil
}
func (m *model) numberKey(msg tea.KeyPressMsg) tea.Cmd {
	d := m.dialog
	if m.navigationKey(msg.String()) == "nav.back" {
		return m.closeDialog()
	}
	if msg.Code == tea.KeyEnter || msg.String() == "ctrl+s" {
		return m.submitDialog()
	}
	key := d.args["key"].(string)
	if key == "n_predict" && msg.String() == "m" {
		d.fields[0].input.SetValue("Max")
		return nil
	}
	delta := 0.0
	switch m.navigationKey(msg.String()) {
	case "nav.up":
		delta = 1
	case "nav.down":
		delta = -1
	case "nav.right":
		delta = 10
	case "nav.left":
		delta = -10
	}
	value, _ := strconv.ParseFloat(d.fields[0].input.Value(), 64)
	unit, lower := 1.0, 1.0
	if key == "temperature" {
		unit, lower = .1, 0
	}
	if key == "top_p" {
		unit, lower = .01, 0
	}
	if key == "monitor_interval_tokens" {
		unit, lower = 64, 1
	}
	if key == "threshold" {
		lower = 0
	}
	if key == "n_predict" {
		unit = 128
	}
	if delta != 0 {
		value = max(lower, value+delta*unit)
		if key == "top_p" {
			value = min(1, value)
		}
		if key == "threshold" {
			value = min(100, value)
		}
		d.fields[0].input.SetValue(strconv.FormatFloat(value, 'f', 2, 64))
	}
	return nil
}
func (m *model) saveNumber(d *dialog) tea.Cmd {
	key, group := d.args["key"].(string), d.args["group"].(string)
	raw := d.fields[0].input.Value()
	if raw == "Max" {
		raw = "-1"
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	if key == "threshold" {
		value /= 100
	}
	var setting any = value
	if key != "temperature" && key != "top_p" && key != "threshold" {
		setting = int(value)
	}
	command := "simulator.configure"
	args := map[string]any{key: setting}
	if d.args["scope"] == "loom-policy" {
		command = "loom-policy.update"
		args = map[string]any{"id": group, key: setting}
	} else if d.args["scope"] == "grow" {
		command = "grow.configure"
		args = map[string]any{"settings": args}
	} else if group != "" {
		merged := map[string]any{}
		if old, ok := m.data.SimulatorConfig[group].(map[string]any); ok {
			for k, v := range old {
				merged[k] = v
			}
		}
		merged[key] = setting
		args = map[string]any{group: merged}
	}
	return m.saveDialog(d, command, args)
}

func samplingLabel(key string) string {
	switch key {
	case "n_predict":
		return "Output tokens"
	case "top_p":
		return "Top-p"
	case "temperature":
		return "Temperature"
	}
	return key
}
func samplingValue(key string, value any) string {
	if key == "n_predict" && fmt.Sprint(value) == "-1" {
		return "Max"
	}
	return fmt.Sprint(value)
}

// Opening controls form a nested page so the main Simulator stays compact.
func (m *model) openOpeningConfig() tea.Cmd {
	mode := m.simString("opening_mode")
	if mode == "" {
		mode = "fixed"
	}
	d := &dialog{kind: "sim-openings", title: "Opening", rows: []row{
		{id: "opening_mode", label: "Mode · " + mode},
	}}
	if mode == "fixed" {
		d.rows = append(d.rows, row{id: "opening", label: "Message", preview: m.simString("opening")})
	} else {
		name := "Use visitor model"
		if alias := m.simString("opening_alias"); alias != "" {
			name = m.modelName(alias)
		}
		d.rows = append(d.rows,
			row{id: "opening_alias", label: "Model · " + name},
			row{id: "opening_prompt", label: "Generation prompt", preview: m.simString("opening_prompt")},
			row{id: "opening_settings", label: "Sampling"},
			row{id: "preview_openings", label: "Preview 3 openings", preview: "Generate openings only; save a labeled preview trace."},
		)
	}
	m.dialog = d
	return nil
}

func (m *model) openingDescription() string {
	if m.simString("opening_mode") == "generated" {
		return "Generated independently for each conversation"
	}
	return m.simString("opening")
}
