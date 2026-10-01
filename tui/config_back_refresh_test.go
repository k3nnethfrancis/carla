package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func applyOperationalTestSave(t *testing.T, m *model, req capturedRequest, purpose string, index int) {
	t.Helper()
	var config map[string]any
	if err := json.Unmarshal(req.Args["config"], &config); err != nil {
		t.Fatal(err)
	}
	for key, value := range config {
		m.data.OperationalPolicies[purpose][index].Config[key] = value
	}
	data, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: data})
}
func TestMonitoringBackRefreshesCountsAndEnabledList(t *testing.T) {
	m := namedOperationalFixture()
	p := &m.data.OperationalPolicies["monitoring"][1]
	p.Config["monitor_mode"] = "diffusion"
	m.data.OperationalPolicies["monitoring"][0].Config["monitor_mode"] = "off"
	dims := p.Config["monitor_dimensions"].([]map[string]any)
	for _, id := range []string{"spiraling", "harmful_language"} {
		dims = append(dims, map[string]any{"id": id, "name": id, "spec": "Spec", "enabled": true, "action": "warn", "color": "amber", "decision": "most_likely", "threshold": .8})
	}
	p.Config["monitor_dimensions"] = dims
	m.openOperationalPolicies("monitoring")
	m.dialog.index = 1
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	root := m.dialog
	chooseBehaviorRow(t, m, "behaviors")
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	for i, r := range m.dialog.rows {
		if r.id == "action" {
			m.dialog.index = i
		}
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyRight}) })
	applyOperationalTestSave(t, m, req, "monitoring", 1)
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != root {
		t.Fatal("back skipped policy")
	}
	if m.dialog.rows[m.dialog.index].id != "behaviors" {
		t.Fatalf("lost focused row index %d rows %+v", m.dialog.index, m.dialog.rows)
	}
	if !strings.Contains(m.dialog.rows[m.dialog.index].label, "2 warn · 1 stop") {
		t.Fatal("stale counts", m.dialog.rows)
	}
	for i, r := range m.dialog.rows {
		if r.id == "mode" {
			m.dialog.index = i
		}
	}
	req = captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace}) })
	applyOperationalTestSave(t, m, req, "monitoring", 1)
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog.kind != "operational-policy-list" || !strings.Contains(m.dialog.rows[m.dialog.index].label, "Draft · Off") {
		t.Fatal("stale policy enabled state", m.dialog)
	}
}
func TestSelectionBackRefreshesBehaviorCountAndList(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("selection")
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	root := m.dialog
	chooseBehaviorRow(t, m, "behaviors")
	p := &m.data.OperationalPolicies["selection"][0]
	bs := p.Config["selection_behaviors"].([]map[string]any)
	p.Config["selection_behaviors"] = append(bs, map[string]any{"id": "voice", "name": "Voice", "spec": "Consistent", "enabled": true})
	data, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", Data: data})
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != root || m.dialog.rows[m.dialog.index].label != "Behaviors · 2" {
		t.Fatal("stale selection count", m.dialog)
	}
	for i, r := range m.dialog.rows {
		if r.id == "selection_enabled" {
			m.dialog.index = i
		}
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace}) })
	applyOperationalTestSave(t, m, req, "selection", 0)
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !strings.Contains(m.dialog.rows[m.dialog.index].label, "Select coherent · On") {
		t.Fatal("stale selection enabled state", m.dialog)
	}
}
