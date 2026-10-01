package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestNamedOperationalPolicyQuickToggleKeys(t *testing.T) {
	for _, purpose := range []string{"monitoring", "selection"} {
		for _, key := range []rune{tea.KeySpace, tea.KeyLeft, tea.KeyRight} {
			m := namedOperationalFixture()
			m.openOperationalPolicies(purpose)
			target := "selection-draft"
			if purpose == "monitoring" {
				m.dialog.index = 1
				target = "draft"
			}
			root := m.dialog
			req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: key}) })
			if req.Command != "operational.policy.save" || string(req.Args["id"]) != `"`+target+`"` {
				t.Fatal(req)
			}
			var changes map[string]any
			json.Unmarshal(req.Args["config"], &changes)
			if purpose == "monitoring" && changes["monitor_mode"] != "diffusion" {
				t.Fatal(changes)
			}
			if purpose == "selection" && changes["selection_enabled"] != true {
				t.Fatal(changes)
			}
			if m.dialog != root || m.data.ActiveOperationalPolicies["monitoring"] != "active" || m.data.SimulatorConfig["monitor_mode"] != "jev" {
				t.Fatal("toggle mutated runtime before backend acknowledgment")
			}
		}
	}
}
func TestOperationalToggleRefreshKeepsFilteredRow(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("monitoring")
	m.filterDialog(tea.KeyPressMsg{Text: "Draft"})
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace}) })
	m.data.OperationalPolicies["monitoring"][0].Config["monitor_mode"] = "off"
	m.data.OperationalPolicies["monitoring"][1].Config["monitor_mode"] = "diffusion"
	m.data.ActiveOperationalPolicies["monitoring"] = "draft"
	m.data.SimulatorConfig["monitor_mode"] = "diffusion"
	m.apply(stateEvent(t, m, req.ID))
	if m.dialog.kind != "operational-policy-list" || m.dialog.query != "Draft" || len(m.dialog.rows) != 1 || m.dialog.rows[0].id != "draft" || !strings.Contains(m.dialog.rows[0].label, " · On") {
		t.Fatal(m.dialog)
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog.kind != "loom-policy" || !strings.Contains(m.dialog.title, "Draft") {
		t.Fatal("Enter did not open policy", m.dialog)
	}
}
func TestOperationalToggleSetupAndEscape(t *testing.T) {
	for _, provider := range []string{"", "jev"} {
		m := namedOperationalFixture()
		m.data.MonitorKeySource = ""
		c := m.data.OperationalPolicies["monitoring"][1].Config
		c["monitor_mode"] = "off"
		c["monitor_provider"] = provider
		m.openOperationalPolicies("monitoring")
		m.dialog.index = 1
		root := m.dialog
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyLeft})
		expected := "loom-policy-pick"
		if provider == "jev" {
			expected = "loom-policy-key"
		}
		if m.dialog.kind != expected || m.pending {
			t.Fatal(m.dialog)
		}
		if provider == "" {
			for _, r := range m.dialog.rows {
				if r.id != "diffusion" && r.id != "jev" {
					t.Fatal("unsupported provider", r)
				}
			}
		}
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.dialog != root || m.dialog.index != 1 || c["monitor_mode"] != "off" || m.data.OperationalPolicies["monitoring"][0].Config["monitor_mode"] != "jev" || m.data.SimulatorConfig["monitor_mode"] != "jev" {
			t.Fatal("Escape lost list or changed config")
		}
	}
}
func TestOperationalSetupFromListSavesNamedTarget(t *testing.T) {
	m := namedOperationalFixture()
	c := m.data.OperationalPolicies["monitoring"][1].Config
	c["monitor_mode"] = "off"
	delete(c, "monitor_provider")
	m.openOperationalPolicies("monitoring")
	m.dialog.index = 1
	root := m.dialog
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyRight})
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter}) })
	if req.Command != "operational.policy.save" || string(req.Args["id"]) != `"draft"` || m.dialog != root {
		t.Fatal(req, m.dialog)
	}
	var changes map[string]any
	json.Unmarshal(req.Args["config"], &changes)
	if changes["monitor_mode"] != "diffusion" {
		t.Fatal(changes)
	}
}
func TestOperationalNewRowDoesNotToggleAndFooterIsAccurate(t *testing.T) {
	m := namedOperationalFixture()
	m.width, m.height = 120, 36
	m.openOperationalPolicies("monitoring")
	m.reflow()
	if !strings.Contains(m.View().Content, "ENTER open") || strings.Contains(m.View().Content, "ENTER toggle") {
		t.Fatal("wrong list hints")
	}
	if m.dialog.rows[0].label != "Live · On" || m.dialog.rows[1].label != "Draft · Off" {
		t.Fatal("rows must show only On/Off")
	}
	m.dialog.index = len(m.dialog.rows) - 1
	if _, ok := m.dialogChoice(); ok {
		t.Fatal("new row advertised toggle")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.pending || m.dialog.kind != "operational-policy-list" {
		t.Fatal("new row toggled")
	}
}

func TestOperationalRootsOfferOnOffAndDeleteWithoutActivation(t *testing.T) {
	for _, purpose := range []string{"monitoring", "selection"} {
		m := namedOperationalFixture()
		m.openOperationalPolicies(purpose)
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		foundDelete := false
		for _, r := range m.dialog.rows {
			if r.id == "activate-policy" || strings.Contains(r.label, "active") {
				t.Fatal("separate activation remained", r)
			}
			if r.id == "delete-policy" {
				foundDelete = true
			}
		}
		if !foundDelete {
			t.Fatal("policy cannot be deleted")
		}
		chooseBehaviorRow(t, m, "delete-policy")
		root := m.dialog.parent
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.dialog != root || m.pending {
			t.Fatal("delete cancel changed state")
		}
	}
}
