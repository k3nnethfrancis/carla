package main

import tea "charm.land/bubbletea/v2"

// Choice rows use the same submit handlers as Enter's picker. This keeps key
// setup, validation and persistence identical for mouse/Enter and quick cycling.
type dialogChoice struct {
	field, current string
	values         []string
	toggle         bool
}

func (m *model) dialogChoice() (dialogChoice, bool) {
	d := m.dialog
	if d == nil || len(d.fields) > 0 || len(d.rows) == 0 {
		return dialogChoice{}, false
	}
	id := d.rows[d.index].id
	c := dialogChoice{field: id}
	switch d.kind {
	case "grow-config":
		c.toggle = id == "selection_enabled"
		return c, c.toggle
	case "loom-policy":
		if id == "monitor_call_mode" {
			c.field, c.current, c.values = id, m.monitorCallMode(), []string{"separate", "bundled"}
			break
		}
		if id != "mode" {
			return c, false
		}
		c.field, c.current, c.values = "monitor_mode", m.simString("monitor_mode"), []string{"off", "diffusion", "jev"}
	case "loom-policy-timing":
		c.toggle = id == "monitor_after_reply" || id == "monitor_during_reply"
		return c, c.toggle
	case "loom-policy-dimension":
		item := m.dimension(d.args["id"].(string))
		switch id {
		case "enabled":
			c.toggle = true
		case "action":
			c.current, c.values = item.Action, []string{"warn", "stop"}
		case "decision":
			c.current, c.values = item.Decision, []string{"most_likely", "threshold"}
		case "color":
			c.current, c.values = item.Color, []string{"amber", "coral", "blue", "violet"}
		default:
			return c, false
		}
	case "sim-openings":
		if id != "opening_mode" {
			return c, false
		}
		c.current, c.values = m.simString(id), []string{"fixed", "generated"}
	default:
		return c, false
	}
	return c, true
}

func (m *model) cycleDialogChoice(step int) tea.Cmd {
	c, ok := m.dialogChoice()
	if !ok || m.pending {
		return nil
	}
	d := m.dialog
	if c.toggle {
		if d.kind == "grow-config" {
			return m.send("policy.configure", map[string]any{"selection_enabled": !m.data.SelectionEnabled})
		}
		return m.submitLoomPolicy(d)
	}
	index := 0
	for i, v := range c.values {
		if v == c.current {
			index = i
			break
		}
	}
	value := c.values[(index+step+len(c.values))%len(c.values)]
	if d.kind == "sim-openings" {
		return m.send("simulator.configure", map[string]any{c.field: value})
	}
	args := map[string]any{"field": c.field}
	if d.args != nil {
		args["id"] = d.args["id"]
	}
	return m.submitLoomPolicy(&dialog{kind: "loom-policy-pick", parent: d, args: args, rows: []row{{id: value}}})
}

func (d *dialog) choicePicker() bool {
	return d.kind == "sim-opening-mode" || (d.kind == "loom-policy-pick" && d.args["field"] != "delete")
}
