package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"strings"
)

type loomDimension struct {
	ID, Name, Spec, Action, Color, Decision string
	Enabled                                 bool
	Threshold                               float64
}

func (d loomDimension) builtin() bool {
	return d.ID == "looping" || d.ID == "spiraling" || d.ID == "harmful_language"
}
func (m *model) dimensions() []loomDimension {
	var out []loomDimension
	b, _ := json.Marshal(m.data.SimulatorConfig["monitor_dimensions"])
	json.Unmarshal(b, &out)
	return out
}
func (m *model) dimension(id string) loomDimension {
	for _, d := range m.dimensions() {
		if d.ID == id {
			return d
		}
	}
	return loomDimension{}
}
func (m *model) openLoomPolicy() tea.Cmd {
	d := &dialog{kind: "loom-policy", title: "Monitoring policy", rows: []row{
		{id: "mode", label: "Monitoring · " + m.simString("monitor_mode"), preview: "Jev via OpenRouter checks document continuations or Character replies with context. Requires OPENROUTER_API_KEY."},
		{id: "model", label: "Model · " + m.simString("monitor_model")},
	}}
	for _, item := range m.dimensions() {
		status := "Off"
		if item.Enabled {
			status = strings.Title(item.Action)
		}
		origin := "custom"
		if item.builtin() {
			origin = "default"
		}
		d.rows = append(d.rows, row{id: item.ID, label: item.Name + " · " + status, preview: origin + " · " + item.Spec})
	}
	interval, ok := m.data.SimulatorConfig["monitor_interval_tokens"].(float64)
	if !ok {
		interval = 512
	}
	cadence := fmt.Sprintf("Every %.0f output tokens", interval)
	if interval == 0 {
		cadence = "End of turn only"
	}
	d.rows = append(d.rows, row{id: "interval", label: "During reply · " + cadence, preview: "Checks character replies while they stream. 0 disables mid-turn checks; completed replies are still checked."})
	d.rows = append(d.rows, row{id: "new", label: "+ New dimension"})
	m.dialog = d
	return nil
}
func (m *model) openDimension(id string) tea.Cmd {
	item := m.dimension(id)
	decision := "Most likely"
	if item.Decision == "threshold" {
		decision = fmt.Sprintf("Probability ≥ %.0f%%", item.Threshold*100)
	}
	d := &dialog{kind: "loom-policy-dimension", title: item.Name, args: map[string]any{"id": id}, rows: []row{
		{id: "enabled", label: fmt.Sprintf("Enabled · %t", item.Enabled)},
		{id: "name", label: "Name · " + item.Name}, {id: "spec", label: "Behavior spec", preview: item.Spec},
		{id: "action", label: "Action · " + item.Action, preview: "Warn highlights a detection. Stop interrupts this conversation, including an in-progress reply."},
		{id: "decision", label: "Decision · " + decision, preview: "Most likely: P(yes) > 50%. Threshold: P(yes) ≥ your cutoff. These are model estimates, not calibrated certainty."},
	}}
	if item.Decision == "threshold" {
		d.rows = append(d.rows, row{id: "threshold", label: fmt.Sprintf("Threshold · %.0f%%", item.Threshold*100)})
	}
	if item.Action == "warn" {
		d.rows = append(d.rows, row{id: "color", label: "Warning color · " + item.Color})
	}
	if !item.builtin() {
		d.rows = append(d.rows, row{id: "delete", label: "Delete dimension"})
	}
	m.dialog = d
	return nil
}

// Policy forms and pickers retain their parent; Escape always moves one level.
func (m *model) submitLoomPolicy(d *dialog) tea.Cmd {
	if len(d.fields) > 0 {
		args := map[string]any{}
		for k, v := range d.args {
			args[k] = v
		}
		if d.kind == "loom-policy-new" {
			args["name"], args["spec"] = d.fields[0].input.Value(), d.fields[1].input.Value()
			return m.saveDialog(d, "loom-policy.add", args)
		}
		field := args["field"].(string)
		delete(args, "field")
		args[field] = d.fields[0].input.Value()
		if field == "monitor_model" {
			return m.saveDialog(d, "simulator.configure", map[string]any{field: args[field]})
		}
		return m.saveDialog(d, "loom-policy.update", args)
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	if d.kind == "loom-policy-pick" {
		field := d.args["field"].(string)
		if field == "monitor_mode" {
			return m.saveDialog(d, "simulator.configure", map[string]any{field: r.id})
		}
		if field == "delete" {
			if r.id != "yes" {
				return m.closeDialog()
			}
			m.dialog = d.parent.parent
			return m.send("loom-policy.delete", map[string]any{"id": d.args["id"]})
		}
		return m.saveDialog(d, "loom-policy.update", map[string]any{"id": d.args["id"], field: r.id})
	}
	if d.kind == "loom-policy" {
		switch r.id {
		case "interval":
			value, ok := m.data.SimulatorConfig["monitor_interval_tokens"].(float64)
			if !ok {
				value = 512
			}
			m.numberConfig("sim", "monitor_interval_tokens", value, "")
			m.dialog.title = "Check every N output tokens · 0 = end only · arrows adjust"
			m.dialog.parent = d
		case "mode":
			m.policyPicker(d, "", "monitor_mode", []string{"off", "jev"})
		case "model":
			m.policyForm(d, "", "monitor_model", m.simString("monitor_model"))
		case "new":
			n := &dialog{kind: "loom-policy-new", title: "New dimension", parent: d, args: map[string]any{}}
			n.add("Name", "")
			n.add("Brief behavior spec", "")
			m.dialog = n
			return n.fields[0].input.Focus()
		default:
			m.openDimension(r.id)
			m.dialog.parent = d
		}
		if m.dialog != nil && len(m.dialog.fields) > 0 {
			return m.dialog.fields[0].input.Focus()
		}
		return nil
	}
	id := d.args["id"].(string)
	item := m.dimension(id)
	switch r.id {
	case "enabled":
		return m.send("loom-policy.update", map[string]any{"id": id, "enabled": !item.Enabled})
	case "name":
		m.policyForm(d, id, r.id, item.Name)
	case "spec":
		m.policyForm(d, id, r.id, item.Spec)
	case "threshold":
		m.numberConfig("loom-policy", "threshold", item.Threshold*100, id)
		m.dialog.title = "Probability threshold · ↑↓ 1% · ←→ 10%"
		m.dialog.parent = d
	case "action":
		m.policyPicker(d, id, r.id, []string{"warn", "stop"})
	case "decision":
		m.policyPicker(d, id, r.id, []string{"most_likely", "threshold"})
	case "color":
		m.policyPicker(d, id, r.id, []string{"amber", "coral", "blue", "violet"})
	case "delete":
		m.policyPicker(d, id, r.id, []string{"no", "yes"})
		m.dialog.title = "Delete " + item.Name + "?"
	}
	if m.dialog != nil && len(m.dialog.fields) > 0 {
		return m.dialog.fields[0].input.Focus()
	}
	return nil
}
func (m *model) policyForm(parent *dialog, id, field, value string) {
	d := &dialog{kind: "loom-policy-field", title: strings.ReplaceAll(field, "_", " "), parent: parent, args: map[string]any{"id": id, "field": field}}
	d.add(d.title, value)
	m.dialog = d
}
func (m *model) policyPicker(parent *dialog, id, field string, values []string) {
	d := &dialog{kind: "loom-policy-pick", title: strings.Title(field), parent: parent, args: map[string]any{"id": id, "field": field}}
	for _, v := range values {
		label := strings.Title(strings.ReplaceAll(v, "_", " "))
		d.rows = append(d.rows, row{id: v, label: label})
	}
	m.dialog = d
}
