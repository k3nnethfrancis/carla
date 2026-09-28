package main

import (
	"charm.land/bubbles/v2/textinput"
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
	mode := "Off"
	if m.simString("monitor_mode") == "jev" {
		mode = "Jev"
	}
	d := &dialog{kind: "loom-policy", title: "Monitoring policy", rows: []row{
		{id: "mode", label: "Monitoring · " + mode, preview: "Off by default. Jev sends monitored text to OpenRouter; API usage may incur charges."},
	}}
	m.dialog = d
	if mode == "Off" {
		return nil
	}
	if m.data.MonitorKeySource == "" {
		d.rows = append(d.rows, row{id: "key", label: "Set up OpenRouter API key", preview: "Complete API key setup to reveal monitoring settings."})
		return nil
	}
	d.rows = append(d.rows,
		row{id: "model", label: "Model · " + m.simString("monitor_model")},
		row{id: "timing", label: "When to check · " + m.monitorTimingSummary(), preview: "Shared with document continuations; Visitor messages are not checked."},
	)
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
	d.rows = append(d.rows, row{id: "key", label: "API key · " + m.data.MonitorKeySource, preview: "Replace the saved key. Keys stay outside workspaces and exported traces."})
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
	if d.kind == "loom-policy-key" {
		key := strings.TrimSpace(d.fields[0].input.Value())
		if key == "" {
			m.status = "Enter an OpenRouter API key"
			return nil
		}
		return m.saveDialog(d, "loom-policy.key", map[string]any{"key": key})
	}
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
			if r.id == "jev" && m.data.MonitorKeySource == "" {
				return m.openMonitorKey(d.parent)
			}
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
	if d.kind == "loom-policy-timing" {
		if r.id == "interval" {
			m.numberConfig("sim", "monitor_interval_tokens", m.monitorInterval(), "")
			m.dialog.title = "Output tokens between checks · ↑↓ 64 · ←→ 640"
			m.dialog.parent = d
			return nil
		}
		enabled := m.monitorTimingEnabled(r.id)
		args := map[string]any{r.id: !enabled}
		if r.id == "monitor_during_reply" && !enabled && m.monitorInterval() == 0 {
			args["monitor_interval_tokens"] = 512
		}
		return m.send("simulator.configure", args)
	}
	if d.kind == "loom-policy" {
		switch r.id {
		case "key":
			return m.openMonitorKey(d)
		case "timing":
			m.openMonitorTiming()
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

func (m *model) monitorInterval() float64 {
	if value, ok := m.data.SimulatorConfig["monitor_interval_tokens"].(float64); ok {
		return value
	}
	return 512
}
func (m *model) monitorTimingEnabled(key string) bool {
	enabled, ok := m.data.SimulatorConfig[key].(bool)
	if !ok {
		enabled = true
	}
	// Older workspaces used zero for end-only monitoring.
	return enabled && (key != "monitor_during_reply" || m.monitorInterval() > 0)
}
func (m *model) monitorTimingSummary() string {
	var parts []string
	if m.monitorTimingEnabled("monitor_after_reply") {
		parts = append(parts, "After reply")
	}
	if m.monitorTimingEnabled("monitor_during_reply") {
		parts = append(parts, "During reply")
	}
	if len(parts) == 0 {
		return "No checks"
	}
	return strings.Join(parts, " + ")
}
func (m *model) openMonitorTiming() tea.Cmd {
	d := &dialog{kind: "loom-policy-timing", title: "When to check", rows: []row{}}
	if m.simString("monitor_mode") != "jev" {
		d.title += " · monitoring off"
	}
	for _, entry := range []struct{ key, label, preview string }{
		{"monitor_after_reply", "After each reply", "Check the completed Character reply (or document continuation). Enter toggles."},
		{"monitor_during_reply", "During a reply", "Check partial output while it streams. Enter toggles; your interval stays saved."},
	} {
		value := "Off"
		if m.monitorTimingEnabled(entry.key) {
			value = "On"
		}
		d.rows = append(d.rows, row{id: entry.key, label: entry.label + " · " + value, preview: entry.preview})
	}
	if m.monitorTimingEnabled("monitor_during_reply") {
		d.rows = append(d.rows, row{id: "interval", label: fmt.Sprintf("Check interval · %.0f output tokens", m.monitorInterval()), preview: "Enter adjusts the interval. One check at a time; if the judge is busy, checks are coalesced rather than queued."})
	}
	m.dialog = d
	return nil
}

func (m *model) openMonitorKey(parent *dialog) tea.Cmd {
	d := &dialog{kind: "loom-policy-key", title: "OpenRouter API key · saved locally for Carla", parent: parent}
	d.add("API key", "")
	d.fields[0].input.EchoMode = textinput.EchoPassword
	d.fields[0].input.EchoCharacter = '•'
	m.dialog = d
	return d.fields[0].input.Focus()
}
