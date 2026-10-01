package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"sort"
	"strings"
)

type operationalPolicy struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

func (m *model) operationalContext() (string, string) {
	starts := []*dialog{m.dialog}
	if m.editing != "" {
		starts = append(starts, m.editReturn)
	}
	for _, start := range starts {
		for d := start; d != nil; d = d.parent {
			purpose, _ := d.args["operational_purpose"].(string)
			id, _ := d.args["operational_id"].(string)
			if purpose != "" && id != "" {
				return purpose, id
			}
			// A list-row toggle targets that named policy. Nested setup dialogs
			// retain this list as parent, including its selected row and filter.
			if d.kind == "operational-policy-list" && len(d.rows) > 0 {
				purpose, _ = d.args["purpose"].(string)
				id = d.rows[d.index].id
				if m.operationalPolicy(purpose, id).ID != "" {
					return purpose, id
				}
			}
		}
	}
	return "", ""
}
func (m *model) operationalPolicy(purpose, id string) operationalPolicy {
	for _, p := range m.data.OperationalPolicies[purpose] {
		if p.ID == id {
			return p
		}
	}
	return operationalPolicy{}
}
func (m *model) operationalConfig() map[string]any {
	purpose, id := m.operationalContext()
	return m.operationalPolicy(purpose, id).Config
}
func (m *model) monitoringConfig() map[string]any {
	purpose, _ := m.operationalContext()
	if purpose == "monitoring" {
		return m.operationalConfig()
	}
	return m.data.SimulatorConfig
}
func (m *model) monitorString(key string) string {
	value, _ := m.monitoringConfig()[key].(string)
	return value
}
func (m *model) selectionValue(key string) any {
	purpose, _ := m.operationalContext()
	if purpose == "selection" {
		return m.operationalConfig()[key]
	}
	switch key {
	case "selection_enabled":
		return m.data.SelectionEnabled
	case "policy_spec":
		return m.data.PolicySpec
	case "policy_prompt":
		return m.data.PolicyPrompt
	case "model_alias":
		return m.data.PolicyModel
	}
	return nil
}
func (m *model) selectionString(key string) string { v, _ := m.selectionValue(key).(string); return v }
func (m *model) selectionEnabled() bool {
	v, _ := m.selectionValue("selection_enabled").(bool)
	return v
}
func (m *model) selectionModelName() string {
	alias := m.selectionString("model_alias")
	for _, model := range m.data.SelectorModels {
		if model.Alias == alias {
			return model.Name
		}
	}
	return alias
}
func (m *model) openOperationalPolicies(purpose string) tea.Cmd {
	d := &dialog{kind: "operational-policy-list", title: strings.Title(purpose) + " policies", args: map[string]any{"purpose": purpose}}
	for _, p := range m.data.OperationalPolicies[purpose] {
		enabled := p.Config["selection_enabled"] == true
		if purpose == "monitoring" {
			enabled = p.Config["monitor_mode"] == "diffusion" || p.Config["monitor_mode"] == "jev"
		}
		state := "Off"
		if enabled {
			state = "On"
		}
		label := p.Name + " · " + state
		if m.data.ActiveOperationalPolicies[purpose] == p.ID {
			label += " · active"
		}
		d.rows = append(d.rows, row{id: p.ID, label: label, preview: "Enter configures judges, behaviors and actions. Space or Left/Right toggles On/Off; this does not change which policy is active."})
	}
	d.rows = append(d.rows, row{id: "new", label: "+ New policy", preview: "Create a named policy from the current settings. Activate it explicitly when ready."})
	m.dialog = d
	return nil
}
func (m *model) openOperationalPolicy(purpose, id string) tea.Cmd {
	// Establish context before rendering the legacy supported settings.
	parent := m.dialog
	anchor := &dialog{args: map[string]any{"operational_purpose": purpose, "operational_id": id}, parent: parent}
	m.dialog = anchor
	if purpose == "monitoring" {
		m.openLoomPolicy()
	} else {
		m.openSelectionConfig()
	}
	m.dialog.args = anchor.args
	m.dialog.parent = parent
	return nil
}
func (m *model) appendOperationalControls(d *dialog) {
	purpose, id := m.operationalContext()
	if id == "" {
		return
	}
	d.args = map[string]any{"operational_purpose": purpose, "operational_id": id}
	p := m.operationalPolicy(purpose, id)
	d.title += " · " + p.Name
	if purpose != "monitoring" || m.monitorString("monitor_mode") == "diffusion" || m.monitorString("monitor_mode") == "jev" || m.monitorString("monitor_provider") == "diffusion" || m.monitorString("monitor_provider") == "jev" {
		d.rows = append(d.rows, row{id: "actions", label: "Actions", preview: "What this policy does with detected behaviors or selected candidates."})
	}
	d.rows = append(d.rows, row{id: "rename-policy", label: "Name · " + p.Name, preview: "Rename this policy without changing its behavior."})
	if m.data.ActiveOperationalPolicies[purpose] != id {
		d.rows = append(d.rows, row{id: "activate-policy", label: "Make active", preview: "Use this policy for future generation runs. Editing alone does not activate it."}, row{id: "delete-policy", label: "Delete policy", preview: "Remove this inactive policy. Existing run results remain saved."})
	}
}
func (m *model) submitOperationalPolicy(d *dialog) tea.Cmd {
	purpose, _ := d.args["purpose"].(string)
	if d.kind == "operational-policy-name" {
		name := strings.TrimSpace(d.fields[0].input.Value())
		if name == "" {
			m.status = "Enter a policy name"
			return nil
		}
		args := map[string]any{"purpose": purpose, "name": name}
		if id, _ := d.args["id"].(string); id != "" {
			args["id"] = id
		}
		return m.saveDialog(d, "operational.policy.save", args)
	}
	if d.kind == "operational-policy-behavior-name" {
		name := strings.TrimSpace(d.fields[0].input.Value())
		if name == "" {
			m.status = "Enter a behavior name"
			return nil
		}
		d.args["name"] = name
		m.editing = "operational-selection-new"
		m.editReturn, m.dialog = d, nil
		m.editor.SetValue("")
		m.focus = 1
		m.status = "Write the behavior spec; save creates the enabled behavior"
		m.reflow()
		return m.editor.Focus()
	}
	r := d.rows[d.index]
	if d.kind == "operational-policy-delete" {
		if r.id != "confirm" {
			return m.closeDialog()
		}
		purpose, id := m.operationalContext()
		m.dialog = d.parent.parent
		return m.send("operational.policy.delete", map[string]any{"purpose": purpose, "id": id})
	}

	if d.kind == "operational-policy-actions" {
		if r.id == "selection" {
			return nil
		}
		m.openOperationalAction(r.id)
		m.dialog.parent = d
		return nil
	}
	if d.kind == "operational-policy-library-monitor" || d.kind == "operational-policy-library-selection" {
		b := m.libraryBehavior(r.id)
		m.dialog = d.parent
		values := map[string]any{"name": b.Name, "spec": b.Spec, "source_id": b.ID, "source_revision": b.Revision, "enabled": true}
		if d.kind == "operational-policy-library-selection" {
			return m.saveSelectionBehavior("", values)
		}
		values["action"], values["color"], values["decision"], values["threshold"] = "warn", "amber", "most_likely", .8
		return m.send("loom-policy.add", values)
	}

	if r.id == "new" {
		m.dialog = &dialog{kind: "operational-policy-name", title: "New " + purpose + " policy", parent: d, args: map[string]any{"purpose": purpose}}
		m.dialog.add("Name", "")
		return m.dialog.fields[0].input.Focus()
	}
	return m.openOperationalPolicy(purpose, r.id)
}
func (m *model) operationalControl(d *dialog, id string) (tea.Cmd, bool) {
	purpose, pid := m.operationalContext()
	switch id {
	case "delete-policy":
		m.dialog = &dialog{kind: "operational-policy-delete", title: "Delete policy?", parent: d, rows: []row{{id: "cancel", label: "Cancel", preview: "Keep this policy."}, {id: "confirm", label: "Delete", preview: "Remove this inactive policy. Existing results remain saved."}}}
		return nil, true

	case "actions":
		m.openOperationalActions()
		m.dialog.parent = d
		return nil, true
	case "activate-policy":
		return m.send("operational.policy.activate", map[string]any{"purpose": purpose, "id": pid}), true
	case "rename-policy":
		m.dialog = &dialog{kind: "operational-policy-name", title: "Policy name", parent: d, args: map[string]any{"purpose": purpose, "id": pid}}
		m.dialog.add("Name", m.operationalPolicy(purpose, pid).Name)
		return m.dialog.fields[0].input.Focus(), true
	}
	return nil, false
}
func (m *model) openOperationalActions() tea.Cmd {
	purpose, _ := m.operationalContext()
	d := &dialog{kind: "operational-policy-actions", title: "Actions"}
	if purpose == "selection" {
		d.rows = []row{{id: "selection", label: "Advance selected candidate", preview: "The judge compares alternatives together. Continue the winner in the next loop; if none qualify, end the loop. Other outputs stay saved."}}
	} else {
		for _, b := range m.dimensions() {
			d.rows = append(d.rows, row{id: b.ID, label: b.Name + " · " + b.Action, preview: "When this behavior is detected, apply its policy action. Disabled behaviors never trigger actions."})
		}
	}
	m.dialog = d
	return nil
}
func (m *model) openOperationalAction(id string) tea.Cmd {
	b := m.dimension(id)
	d := &dialog{kind: "loom-policy-action", title: b.Name + " · action", args: map[string]any{"id": id}, rows: []row{{id: "action", label: "Action · " + b.Action, preview: "Warn highlights detection. Stop interrupts this generation."}}}
	if b.Action == "warn" {
		d.rows = append(d.rows, row{id: "color", label: "Warning color · " + b.Color, preview: "Highlight color used when this policy detects the behavior."})
	}
	m.dialog = d
	return nil
}

// Route editor mutations to the named policy being edited. Active runtime state
// is projected only by the backend when that policy is active.
func (m *model) routeOperationalMutation(command string, args map[string]any) (string, map[string]any) {
	purpose, id := m.operationalContext()
	if id == "" {
		return command, args
	}
	p := m.operationalPolicy(purpose, id)
	changes := map[string]any{}
	switch command {
	case "simulator.configure":
		if purpose != "monitoring" {
			return command, args
		}
		for k, v := range args {
			if !strings.HasPrefix(k, "monitor_") {
				return command, args
			}
			changes[k] = v
		}
	case "policy.configure", "configure":
		if purpose != "selection" {
			return command, args
		}
		for key := range args {
			if key != "selection_enabled" && key != "policy_spec" && key != "policy_prompt" && key != "selection_behaviors" && key != "model_alias" {
				return command, args
			}
		}
		for k, v := range args {
			changes[k] = v
		}
		if spec, ok := changes["policy_spec"]; ok && m.operationalBehaviorID() != "" {
			bs := m.selectionBehaviors()
			for _, b := range bs {
				if b["id"] == m.operationalBehaviorID() {
					b["spec"] = spec
				}
			}
			delete(changes, "policy_spec")
			changes["selection_behaviors"] = bs
		}
	case "grow.selector":
		if purpose != "selection" {
			return command, args
		}
		changes["model_alias"] = args["alias"]
	case "loom-policy.update", "loom-policy.delete", "loom-policy.add":
		if purpose != "monitoring" {
			return command, args
		}
		var dimensions []map[string]any
		b, _ := json.Marshal(p.Config["monitor_dimensions"])
		json.Unmarshal(b, &dimensions)
		if command == "loom-policy.add" {
			item := map[string]any{}
			for k, v := range args {
				item[k] = v
			}
			dimensions = append(dimensions, item)
		} else {
			result := dimensions[:0]
			for _, b := range dimensions {
				if b["id"] == args["id"] {
					if command == "loom-policy.delete" {
						continue
					}
					for k, v := range args {
						b[k] = v
					}
				}
				result = append(result, b)
			}
			dimensions = result
		}
		changes["monitor_dimensions"] = dimensions
	default:
		return command, args
	}
	return "operational.policy.save", map[string]any{"purpose": purpose, "id": id, "name": p.Name, "config": changes}
}

func (m *model) operationalBehaviorID() string {
	for _, start := range []*dialog{m.dialog, m.editReturn} {
		for d := start; d != nil; d = d.parent {
			if id, _ := d.args["selection_behavior"].(string); id != "" {
				return id
			}
		}
	}
	return ""
}
func (m *model) selectionBehaviors() []map[string]any {
	var out []map[string]any
	b, _ := json.Marshal(m.operationalConfig()["selection_behaviors"])
	json.Unmarshal(b, &out)
	if len(out) == 0 {
		out = []map[string]any{{"id": "criteria", "name": "Selection criteria", "spec": m.selectionString("policy_spec"), "enabled": true}}
	}
	return out
}
func (m *model) selectionSpec() string {
	id := m.operationalBehaviorID()
	for _, b := range m.selectionBehaviors() {
		if b["id"] == id {
			v, _ := b["spec"].(string)
			return v
		}
	}
	return m.selectionString("policy_spec")
}
func (m *model) saveSelectionBehavior(id string, changes map[string]any) tea.Cmd {
	bs := m.selectionBehaviors()
	found := false
	for _, b := range bs {
		if b["id"] == id {
			for k, v := range changes {
				b[k] = v
			}
			found = true
		}
	}
	if !found {
		b := map[string]any{"id": id, "enabled": true}
		for k, v := range changes {
			b[k] = v
		}
		bs = append(bs, b)
	}
	return m.send("configure", map[string]any{"selection_behaviors": bs})
}

func (m *model) saveOperationalSelectionDraft(text string) tea.Cmd {
	if strings.TrimSpace(text) == "" {
		m.status = "Write a behavior spec before saving"
		return nil
	}
	d := m.editReturn
	name, _ := d.args["name"].(string)
	bs := m.selectionBehaviors()
	bs = append(bs, map[string]any{"name": name, "spec": text, "enabled": true})
	return m.submitEditor("configure", map[string]any{"selection_behaviors": bs})
}

// Present policy structure before optional scheduling and management actions.
func orderOperationalRows(d *dialog) {
	if d.args["operational_id"] == nil {
		return
	}
	order := map[string]int{"rename-policy": 0, "mode": 1, "selection_enabled": 1, "key": 2, "judges": 2, "actions": 3, "timing": 4, "activate-policy": 5, "delete-policy": 6}
	sort.SliceStable(d.rows, func(i, j int) bool { return order[d.rows[i].id] < order[d.rows[j].id] })
}

func (m *model) toggleOperationalPolicy() tea.Cmd {
	purpose, id := m.operationalContext()
	if id == "" {
		return nil
	}
	if purpose == "monitoring" {
		return m.toggleMonitoring(m.dialog)
	}
	return m.send("policy.configure", map[string]any{"selection_enabled": !m.selectionEnabled()})
}
