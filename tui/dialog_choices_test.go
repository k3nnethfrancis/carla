package main

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func TestQuickMonitorChoices(t *testing.T) {
	for _, key := range []rune{tea.KeySpace, tea.KeyLeft, tea.KeyRight} {
		m := policyFixture()
		m.openLoomPolicy()
		req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: key}) })
		if req.Command != "simulator.configure" || string(req.Args["monitor_mode"]) != `"off"` || m.dialog.kind != "loom-policy" || m.dialog.query != "" {
			t.Fatal(req, m.dialog)
		}
	}
	m := policyFixture()
	m.data.MonitorKeySource = ""
	m.data.SimulatorConfig["monitor_mode"] = "off"
	m.openLoomPolicy()
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if m.dialog.kind != "loom-policy-key" || m.simString("monitor_mode") != "off" {
		t.Fatal("quick cycle bypassed key setup")
	}
}

func TestHeartbeatQuickTogglePreservesFilter(t *testing.T) {
	m := policyFixture()
	m.openMonitorTiming()
	m.filterDialog(tea.KeyPressMsg{Text: "During"})
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}) })
	if string(req.Args["monitor_during_reply"]) != "false" {
		t.Fatal(req)
	}
	m.data.SimulatorConfig["monitor_during_reply"] = false
	m.refreshConfig()
	if m.dialog.query != "During" || len(m.dialog.rows) != 1 || !strings.Contains(m.dialog.rows[0].label, "Off") || m.dialog.title != "Heartbeat" {
		t.Fatal(m.dialog)
	}
}

func TestQuickCycleDirectionAndTextEntry(t *testing.T) {
	m := policyFixture()
	m.openDimension("looping")
	for i, r := range m.dialog.rows {
		if r.id == "color" {
			m.dialog.index = i
		}
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyLeft}) })
	if string(req.Args["color"]) != `"violet"` {
		t.Fatal(req)
	}
	m.pending = false
	m.policyForm(m.dialog, "looping", "name", "A")
	m.dialog.fields[0].input.Focus()
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !strings.Contains(m.dialog.fields[0].input.Value(), " ") {
		t.Fatal("space stolen from text field")
	}
}
