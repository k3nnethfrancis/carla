package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"sort"
	"strconv"
)

func settingHelp(field int) string {
	switch field {
	case 0:
		return "Max uses remaining context after the prompt."
	case 2:
		return "Top-p range: 0–1. Step: 0.01."
	case 3:
		return "Default uses the model's native context."
	}
	return "↑↓ small steps · ←→ larger steps"
}

// Settings are pickers/steppers, never text entry. The existing field values
// remain the submission payload, so validation has one backend owner.
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
	case "nav.left":
		step = -10
	case "nav.right":
		step = 10
	}
	if step == 0 {
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
	if d.field != 0 && d.field != 3 {
		return
	}
	special := "Max"
	if d.field == 3 {
		special = "Default"
	}
	capacity := m.data.ModelContext
	if capacity == 0 || d.field == 3 {
		capacity = m.data.NativeContext
	}
	values := []int{128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144, 524288, 1048576}
	if capacity > 0 {
		values = append(values, capacity)
	}
	if current, err := strconv.Atoi(d.previous); err == nil {
		values = append(values, current)
	}
	sort.Ints(values)
	preview := settingHelp(d.field)
	label := special
	if d.field == 3 && m.data.NativeContext > 0 {
		label += " · " + tokenNumber(m.data.NativeContext)
	}
	d.rows = []row{{id: special, label: label, preview: preview}}
	seen := map[int]bool{}
	for _, n := range values {
		if n <= 0 || seen[n] || (capacity > 0 && n > capacity && strconv.Itoa(n) != d.previous) {
			continue
		}
		seen[n] = true
		d.rows = append(d.rows, row{id: strconv.Itoa(n), label: tokenNumber(n), preview: preview})
	}
	d.index = 0
	for i, r := range d.rows {
		if r.id == d.previous {
			d.index = i
		}
	}
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
