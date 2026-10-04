package main

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func selectCalls(m *model) {
	m.openMonitorJudge()
	for i, r := range m.dialog.rows {
		if r.id == "monitor_call_mode" {
			m.dialog.index = i
			return
		}
	}
}
func TestMonitorCallWarningAllPaths(t *testing.T) {
	for _, key := range []rune{tea.KeyEnter, tea.KeySpace, tea.KeyLeft, tea.KeyRight} {
		m := policyFixture()
		selectCalls(m)
		root := m.dialog
		if !strings.Contains(root.rows[root.index].label, "Separate") {
			t.Fatal("missing separate default")
		}
		m.dialogKey(tea.KeyPressMsg{Code: key})
		if key == tea.KeyEnter {
			m.dialog.index = 1
			m.submitDialog()
		}
		if m.dialog.kind != "loom-policy-bundle-confirm" || m.dialog.index != 0 || m.monitorCallMode() != "separate" || m.pending {
			t.Fatalf("key %v bypassed warning: %#v", key, m.dialog)
		}
		warning := m.dialog
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.dialog != warning.parent || m.monitorCallMode() != "separate" {
			t.Fatal("escape did not unwind without save")
		}
		m.confirmBundledCalls(m.dialog)
		m.submitDialog()
		if m.dialog.kind == "loom-policy-bundle-confirm" || m.pending {
			t.Fatal("cancel changed state")
		}
		m.confirmBundledCalls(m.dialog)
		m.dialog.index = 1
		req := captureCommand(t, m, m.submitDialog)
		if req.Command != "simulator.configure" || string(req.Args["monitor_call_mode"]) != `"bundled"` || m.dialog != root {
			t.Fatal(req, m.dialog)
		}
		m.data.SimulatorConfig["monitor_call_mode"] = "bundled"
		m.pending = false
		m.refreshConfig()
		if !strings.Contains(m.dialog.rows[m.dialog.index].label, "Bundled") {
			t.Fatal("saved setting not refreshed")
		}
		m.submitDialog()
		if m.dialog.index != 1 {
			t.Fatal("picker not on saved value")
		}
		m.submitDialog()
		if m.dialog.kind != "loom-policy-judge" || m.pending {
			t.Fatal("unchanged bundled asked again")
		}
		req = captureCommand(t, m, func() tea.Cmd { return m.cycleDialogChoice(1) })
		if string(req.Args["monitor_call_mode"]) != `"separate"` || m.dialog.kind != "loom-policy-judge" {
			t.Fatal("separate required warning", req)
		}
	}
}
func TestMonitorCallSetupGate(t *testing.T) {
	for _, provider := range []string{"off", "jev"} {
		m := policyFixture()
		m.data.MonitorKeySource = ""
		m.data.SimulatorConfig["monitor_mode"] = provider
		m.openLoomPolicy()
		for _, r := range m.dialog.rows {
			if r.id == "monitor_call_mode" {
				t.Fatal("setup gate bypassed")
			}
		}
	}
}
func TestMonitorPartialStatusKeepsDetailsInInspector(t *testing.T) {
	text := monitorSummary(monitorResult{Status: "partial", Error: "spiraling: timeout", Scores: map[string]float64{"looping": .9}})
	if strings.Contains(text, "timeout") || strings.Contains(text, "90%") {
		t.Fatal(text)
	}
	for _, want := range []string{"monitoring incomplete"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
}
