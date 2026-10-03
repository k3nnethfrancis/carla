package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strconv"
)

func settingHelp(field int) string {
	switch field {
	case 0:
		return "Max uses remaining context after the prompt."
	case 2:
		return "Top-p range: 0–1. Step: 0.01."
	case 3:
		return "Max uses the model's native context; larger contexts need more memory."
	}
	return "Type a value · ↑↓ adjust · ←→ move cursor"
}

// Browse settings, then Enter to edit a value using normal text input.
func (m *model) settingsKey(msg tea.KeyPressMsg) tea.Cmd {
	d := m.dialog
	key := m.navigationKey(msg.String())
	if msg.String() == "ctrl+s" || m.boundAction(msg.String(), "editor") == "save" {
		if d.adjusting {
			m.finishSetting(false)
		}
		return m.submitDialog()
	}
	if key == "nav.back" {
		if d.adjusting {
			m.finishSetting(true)
		} else {
			return m.closeDialog()
		}
		return nil
	}
	if key == "nav.enter" {
		if d.adjusting {
			m.finishSetting(false)
		} else {
			m.startSetting()
		}
		return nil
	}
	step := 0
	switch key {
	case "nav.up", "nav.prev":
		step = -1
	case "nav.down", "nav.next":
		step = 1

	}
	if step == 0 {
		if d.adjusting {
			if msg.String() == "m" && (d.field == 0 || d.field == 3) {
				d.fields[d.field].input.SetValue("Max")
				return nil
			}
			var cmd tea.Cmd
			d.fields[d.field].input, cmd = d.fields[d.field].input.Update(msg)
			return cmd
		}
		return nil
	}
	if !d.adjusting {
		if step == -1 || step == 1 {
			d.field = (d.field + step + len(d.fields)) % len(d.fields)
		}
		return nil
	}
	if len(d.rows) > 0 {
		d.index = max(0, min(len(d.rows)-1, d.index+step))
		return nil
	}
	value, _ := strconv.ParseFloat(d.fields[d.field].input.Value(), 64)
	unit, lower := 1.0, 1.0
	if d.field == 1 {
		unit, lower = .1, 0
	}
	if d.field == 2 {
		unit, lower = .01, 0
	}
	// Up increases numeric steppers; Down decreases.
	if step == 1 || step == -1 {
		step = -step
	}
	value = max(lower, value+float64(step)*unit)
	if d.field == 2 {
		value = min(1, value)
	}
	if d.field == 1 || d.field == 2 {
		d.fields[d.field].input.SetValue(strconv.FormatFloat(value, 'f', 2, 64))
	} else {
		d.fields[d.field].input.SetValue(strconv.FormatFloat(value, 'f', 0, 64))
	}
	return nil
}
func (m *model) startSetting() {
	d := m.dialog
	d.adjusting = true
	d.previous = d.fields[d.field].input.Value()
	d.fields[d.field].input.Focus()
}
func (m *model) finishSetting(cancel bool) {
	d := m.dialog
	if cancel {
		d.fields[d.field].input.SetValue(d.previous)
	} else if len(d.rows) > 0 {
		d.fields[d.field].input.SetValue(d.rows[d.index].id)
	}
	d.adjusting = false
	d.rows = nil
	d.index = 0
	m.status = fmt.Sprintf("%s · CTRL+S saves settings", d.fields[d.field].label)
}
