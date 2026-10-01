package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
)

func (d loomDimension) args() map[string]any {
	return map[string]any{"name": d.Name, "spec": d.Spec, "enabled": d.Enabled, "action": d.Action, "color": d.Color, "decision": d.Decision, "threshold": d.Threshold}
}

func (m *model) openBehaviors() {
	d := &dialog{kind: "loom-policy-behaviors", title: "Behaviors"}
	for _, item := range m.dimensions() {
		enabled := "Disabled"
		if item.Enabled {
			enabled = "Enabled"
		}
		detection := "most likely"
		if item.Decision == "threshold" {
			detection = fmt.Sprintf("≥ %.0f%%", item.Threshold*100)
		}
		d.rows = append(d.rows, row{id: item.ID, label: item.Name + " · " + enabled, preview: "Detection: " + detection + ". " + item.Spec})
	}
	d.rows = append(d.rows, row{id: "new", label: "+ New behavior", preview: "Configure its spec and detection rule before saving. Policy actions are configured separately."})
	d.rows = append(d.rows, row{id: "library", label: "From library", preview: "Copy a saved behavior definition into this judge."})
	m.dialog = d
}

// New behavior edits stay local until Create. Existing behavior changes retain
// the normal backend validation and snapshot-refresh path.
func (m *model) updateBehavior(args map[string]any) tea.Cmd {
	if args["id"] != "draft" {
		return m.send("loom-policy.update", args)
	}
	if m.behaviorDraft == nil {
		return nil
	}
	values := m.behaviorDraft.args()
	for k, v := range args {
		values[k] = v
	}
	data, _ := json.Marshal(values)
	_ = json.Unmarshal(data, m.behaviorDraft)
	m.refreshConfig()
	return nil
}
