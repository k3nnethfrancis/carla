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
	b, _ := json.Marshal(m.monitoringConfig()["monitor_dimensions"])
	json.Unmarshal(b, &out)
	return out
}
func (m *model) dimension(id string) loomDimension {
	if id == "draft" && m.behaviorDraft != nil {
		return *m.behaviorDraft
	}
	for _, d := range m.dimensions() {
		if d.ID == id {
			return d
		}
	}
	return loomDimension{}
}
func (m *model) openLoomPolicy() tea.Cmd {
	provider := m.monitorString("monitor_mode")
	mode := "Off"
	if provider == "diffusion" || provider == "jev" {
		mode = "On"
	}
	d := &dialog{kind: "loom-policy", title: "Monitoring policy", rows: []row{
		{id: "mode", label: "Monitoring · " + mode, preview: "Enable or disable monitoring. The judge and its behaviors stay saved when Off. First-time setup asks you to choose a model."},
	}}
	defer orderOperationalRows(d)
	m.appendOperationalControls(d)
	m.dialog = d
	if mode == "Off" {
		provider = m.monitorString("monitor_provider")
	}
	if provider != "jev" && provider != "diffusion" {
		return nil
	}
	if provider == "jev" && m.data.MonitorKeySource == "" {
		d.rows = append(d.rows, row{id: "key", label: "Set up OpenRouter API key", preview: "Complete API key setup to reveal monitoring settings."})
		return nil
	}
	d.rows = append(d.rows,
		row{id: "timing", label: "Heartbeat · " + m.monitorTimingSummary(), preview: "When this policy runs: during streaming output, after completed replies, or both."},
		row{id: "judges", label: "Judges · 1", preview: "Open the monitoring model, its call mode and behaviors. Monitoring currently uses one classifier judge."},
	)
	return nil
}

func (m *model) monitorJudgeName() string {
	provider := m.monitorString("monitor_mode")
	if provider == "off" || provider == "" {
		provider = m.monitorString("monitor_provider")
	}
	if provider == "diffusion" {
		return "DiffusionGemma (classifier)"
	}
	return "Jev (classifier)"
}
func (m *model) openMonitorJudges() tea.Cmd {
	m.dialog = &dialog{kind: "loom-policy-judges", title: "Monitoring judges", rows: []row{
		{id: "judge", label: m.monitorJudgeName(), preview: "Assesses enabled behaviors and applies their Warn or Stop actions."},
	}}
	return nil
}
func (m *model) openMonitorJudge() tea.Cmd {
	d := &dialog{kind: "loom-policy-judge", title: m.monitorJudgeName(), rows: []row{
		{id: "mode", label: "Model · " + m.monitorJudgeName(), preview: "Choose the classifier. DiffusionGemma runs locally; Jev uses OpenRouter and requires an API key."},
		{id: "monitor_call_mode", label: "Call mode · " + strings.Title(m.monitorCallMode()), preview: "Separate sends one request per behavior. Bundled checks all enabled behaviors in one request."},
		{id: "behaviors", label: "Behaviors · " + m.behaviorCounts(), preview: "Specs and detection rules for this judge. Warn/Stop and colors live in the policy’s Actions."},
	}}
	if m.monitorJudgeName() == "Jev (classifier)" {
		d.rows = append(d.rows, row{id: "key", label: "API key · " + m.data.MonitorKeySource, preview: "Replace the saved OpenRouter key. Keys stay outside workspaces and exported traces."})
	}
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
		{id: "enabled", label: "Enabled · " + map[bool]string{true: "On", false: "Off"}[item.Enabled], preview: "Off skips this behavior entirely; its detection and action settings remain saved."},
		{id: "name", label: "Name · " + item.Name, preview: "The behavior label shown in policy results and warnings."}, {id: "spec", label: "Behavior spec", preview: "Defines what the judge should detect. " + item.Spec},
		{id: "decision", label: "Detection rule · " + decision, preview: "Determines when the behavior counts as detected. Most likely: estimated probability > 50%. Threshold: probability ≥ your cutoff. Action then decides Warn or Stop; probabilities are not calibrated confidence."},
	}}
	if item.Decision == "threshold" {
		d.rows = append(d.rows, row{id: "threshold", label: fmt.Sprintf("Threshold · %.0f%%", item.Threshold*100), preview: "Minimum estimated probability that triggers this behavior’s action."})
	}

	if id == "draft" {
		d.title = "New behavior"
		d.rows = append(d.rows, row{id: "create", label: "Create behavior", preview: "Save the name, full spec and all settings shown here."})
	} else if !item.builtin() {
		d.rows = append(d.rows, row{id: "delete", label: "Delete behavior", preview: "Remove this custom behavior from the monitoring policy. Confirmation is required."})
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
		args := map[string]any{"key": key}
		if _, id := m.operationalContext(); id != "" {
			args["activate"] = false
			args["policy_id"] = id
			if d.parent != nil && d.parent.kind == "loom-policy-judge" && m.monitorString("monitor_mode") == "off" {
				args["preserve_disabled"] = true
			}
		}
		return m.saveDialog(d, "loom-policy.key", args)
	}
	if len(d.fields) > 0 {
		args := map[string]any{}
		for k, v := range d.args {
			args[k] = v
		}

		field := args["field"].(string)
		delete(args, "field")
		args[field] = d.fields[0].input.Value()
		if field == "monitor_model" || field == "monitor_local_model" {
			return m.saveDialog(d, "simulator.configure", map[string]any{field: args[field]})
		}
		return m.saveDialog(d, "loom-policy.update", args)
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	if d.kind == "loom-policy-bundle-confirm" {
		if r.id != "confirm" {
			return m.closeDialog()
		}
		// Return to settings after confirmation; Escape retains the picker.
		parent := d.parent
		if parent.kind == "loom-policy-pick" {
			parent = parent.parent
		}
		m.dialog = parent
		return m.send("simulator.configure", map[string]any{"monitor_call_mode": "bundled"})
	}
	if d.kind == "loom-policy-pick" {
		field := d.args["field"].(string)
		if field == "monitor_call_mode" {
			if r.id == m.monitorCallMode() {
				return m.closeDialog()
			}
			if r.id == "bundled" {
				parent := d
				if m.dialog != d {
					parent = d.parent
				}
				m.confirmBundledCalls(parent)
				return nil
			}
			return m.saveDialog(d, "simulator.configure", map[string]any{field: r.id})
		}
		if field == "monitor_mode" {
			if r.id == "jev" && m.data.MonitorKeySource == "" {
				return m.openMonitorKey(d.parent)
			}
			args := map[string]any{field: r.id}
			if d.parent != nil && d.parent.kind == "loom-policy-judge" && m.monitorString("monitor_mode") == "off" {
				args[field] = "off"
				args["monitor_provider"] = r.id
			}
			if field == "monitor_mode" && r.id == "diffusion" {
				args["monitor_local_url"] = "auto"
			}
			return m.saveDialog(d, "simulator.configure", args)
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
	if d.kind == "loom-policy-behaviors" {
		if r.id == "library" {
			m.openBehaviorLibraryPicker(d, "operational-policy-library-monitor", nil)
			return nil
		}
		if r.id == "new" {
			m.behaviorDraft = &loomDimension{ID: "draft", Enabled: true, Action: "warn", Color: "amber", Decision: "most_likely", Threshold: .8}
			m.openDimension("draft")
		} else {
			m.openDimension(r.id)
		}
		m.dialog.parent = d
		return nil
	}
	if d.kind == "loom-policy" || d.kind == "loom-policy-judges" || d.kind == "loom-policy-judge" {
		if cmd, ok := m.operationalControl(d, r.id); ok {
			return cmd
		}
		switch r.id {
		case "judges":
			m.openMonitorJudges()
			m.dialog.parent = d
		case "judge":
			m.openMonitorJudge()
			m.dialog.parent = d
		case "key":
			return m.openMonitorKey(d)
		case "behaviors":
			m.openBehaviors()
			m.dialog.parent = d
		case "timing":
			m.openMonitorTiming()
			m.dialog.parent = d
		case "mode":
			if d.kind == "loom-policy" {
				return m.toggleMonitoring(d)
			}
			m.policyPicker(d, "", "monitor_mode", []string{"diffusion", "jev"})
		case "monitor_call_mode":
			m.policyPicker(d, "", r.id, []string{"separate", "bundled"})
			if m.monitorCallMode() == "bundled" {
				m.dialog.index = 1
			}
		case "monitor_local_model":
			m.policyForm(d, "", r.id, m.monitorString(r.id))
		case "model":
			m.policyForm(d, "", "monitor_model", m.monitorString("monitor_model"))

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
		return m.updateBehavior(map[string]any{"id": id, "enabled": !item.Enabled})
	case "name":
		m.policyForm(d, id, r.id, item.Name)
	case "spec":
		m.behaviorEditID = id
		cmd := m.beginEdit("monitor_spec")
		m.editReturn, m.dialog = d, nil
		m.reflow()
		return cmd
	case "create":
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Spec) == "" {
			m.status = "Add a name and behavior spec before creating"
			d.rows[d.index].preview = m.status
			return nil
		}
		if _, pid := m.operationalContext(); pid != "" {
			m.dialog = d.parent
			return m.send("loom-policy.add", item.args())
		}
		request, cmd := m.dispatch("loom-policy.add", item.args())
		if cmd != nil {
			m.behaviorCreating = request
		}
		return cmd
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
	if field == "monitor_local_model" {
		d.title = "Local judge model"
	}
	d.add(d.title, value)
	m.dialog = d
}
func (m *model) policyPicker(parent *dialog, id, field string, values []string) {
	d := &dialog{kind: "loom-policy-pick", title: strings.Title(field), parent: parent, args: map[string]any{"id": id, "field": field}}
	for _, v := range values {
		label := strings.Title(strings.ReplaceAll(v, "_", " "))
		preview := ""
		if field == "monitor_mode" {
			d.title = "Monitoring model"
			label = map[string]string{"diffusion": "DiffusionGemma (classifier)", "jev": "Jev (classifier)", "off": "Off"}[v]
			preview = "Local classifier; Carla manages its inference server."
			if v == "jev" {
				preview = "Hosted classifier via OpenRouter. Requires an API key and incurs API costs."
			}
		}
		if field == "monitor_call_mode" {
			d.title = "Call mode"
			preview = "One request per behavior."
			if v == "bundled" {
				preview = "All enabled behaviors in one request."
			}
		}
		if field == "decision" {
			d.title = "Detection rule"
			if v == "most_likely" {
				preview = "Detected when the judge estimates probability above 50%. The configured action then runs."
			} else {
				preview = "Detected at or above your probability cutoff. The configured action then runs."
			}
		}
		d.rows = append(d.rows, row{id: v, label: label, preview: preview})
	}
	m.dialog = d
}

func (m *model) monitorInterval() float64 {
	if value, ok := m.monitoringConfig()["monitor_interval_tokens"].(float64); ok {
		return value
	}
	return 512
}
func (m *model) monitorTimingEnabled(key string) bool {
	enabled, ok := m.monitoringConfig()[key].(bool)
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
	d := &dialog{kind: "loom-policy-timing", title: "Heartbeat", rows: []row{}}
	if m.monitorString("monitor_mode") == "off" {
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
		d.rows = append(d.rows, row{id: "interval", label: fmt.Sprintf("Interval · %.0f output tokens", m.monitorInterval()), preview: "Enter adjusts the interval. One check at a time; if the judge is busy, checks are coalesced rather than queued."})
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

func monitorLabel(provider string) string {
	switch provider {
	case "diffusion":
		return "DiffusionGemma (local)"
	case "jev":
		return "Jev (OpenRouter)"
	default:
		return "Off"
	}
}

func (m *model) monitorCallMode() string {
	if m.monitorString("monitor_call_mode") == "bundled" {
		return "bundled"
	}
	return "separate"
}

func (m *model) confirmBundledCalls(parent *dialog) {
	const warning = "Bundled calls may reduce cost and latency, but asking behaviors together can change scores or miss detections. Switch to bundled?"
	m.dialog = &dialog{kind: "loom-policy-bundle-confirm", title: "Switch to bundled calls?", parent: parent, rows: []row{
		{id: "cancel", label: "Keep separate", preview: warning},
		{id: "confirm", label: "Use bundled", preview: warning},
	}}
}

func (m *model) behaviorCounts() string {
	warn, stop, off := 0, 0, 0
	for _, behavior := range m.dimensions() {
		if !behavior.Enabled {
			off++
		} else if behavior.Action == "stop" {
			stop++
		} else {
			warn++
		}
	}
	return fmt.Sprintf("%d warn · %d stop · %d off", warn, stop, off)
}

// Enabled is policy-level; changing the classifier belongs to its judge.
func (m *model) toggleMonitoring(parent *dialog) tea.Cmd {
	if provider := m.monitorString("monitor_mode"); provider == "diffusion" || provider == "jev" {
		return m.send("simulator.configure", map[string]any{"monitor_mode": "off"})
	}
	provider := m.monitorString("monitor_provider")
	if provider != "diffusion" && provider != "jev" {
		m.policyPicker(parent, "", "monitor_mode", []string{"diffusion", "jev"})
		return nil
	}
	if provider == "jev" && m.data.MonitorKeySource == "" {
		return m.openMonitorKey(parent)
	}
	return m.send("simulator.configure", map[string]any{"monitor_mode": provider})
}
