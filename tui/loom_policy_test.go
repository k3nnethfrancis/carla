package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func policyFixture() *model {
	m := fixture()
	m.data.SimulatorConfig = map[string]any{"monitor_mode": "off", "monitor_model": "jev-latest", "monitor_dimensions": []any{
		map[string]any{"id": "looping", "name": "Looping", "spec": "Repeating without development", "enabled": true, "action": "warn", "color": "amber", "decision": "most_likely", "threshold": .8},
		map[string]any{"id": "custom_1", "name": "Drift", "spec": "Loses voice", "enabled": false, "action": "stop", "color": "coral", "decision": "threshold", "threshold": .9},
	}}
	return m
}
func TestLoomPolicyNestedNavigationAndProtectedDefaults(t *testing.T) {
	m := policyFixture()
	m.width, m.height = 120, 36
	m.perform("loom-policy")
	root := m.dialog
	root.index = 2
	m.submitDialog()
	if m.dialog.kind != "loom-policy-dimension" || m.dialog.parent != root {
		t.Fatal("dimension lost parent")
	}
	for _, r := range m.dialog.rows {
		if r.id == "delete" {
			t.Fatal("default can be deleted")
		}
	}
	m.dialog.index = 4
	m.submitDialog()
	if m.dialog.kind != "loom-policy-pick" {
		t.Fatal("missing decision picker")
	}
	m.closeDialog()
	if m.dialog.kind != "loom-policy-dimension" {
		t.Fatal("Escape skipped dimension")
	}
	m.closeDialog()
	if m.dialog != root {
		t.Fatal("Escape skipped policy")
	}
	root.index = 3
	m.submitDialog()
	found := false
	for _, r := range m.dialog.rows {
		if r.id == "delete" {
			found = true
		}
	}
	if !found {
		t.Fatal("custom deletion missing")
	}
	m.openLoomPolicy()
	m.dialog.index = 1
	m.submitDialog()
	if !m.dialog.fields[0].input.Focused() {
		t.Fatal("model field not focused")
	}
	m.closeDialog()
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	if len(m.dialog.fields) != 2 || m.dialog.kind != "loom-policy-new" {
		t.Fatal("missing creation form")
	}
	for _, size := range [][2]int{{80, 24}, {120, 36}, {60, 18}} {
		m.width, m.height = size[0], size[1]
		m.reflow()
		_ = m.View()
	}
}
func TestPolicyPulseIsEventDrivenAndExpires(t *testing.T) {
	m := policyFixture()
	data := json.RawMessage(`{"run":"r","conversation":0,"turn":1,"id":"looping","name":"Looping","action":"warn","color":"amber"}`)
	if m.detectPolicy(data) == nil {
		t.Fatal("no animation tick")
	}
	key := conversationKey("r", 0)
	first := m.policyPulses[key]
	if m.detectPolicy(data) != nil || m.policyPulses[key].Started != first.Started {
		t.Fatal("duplicate pulse")
	}
	if m.advancePolicyPulse(first.Started.Add(32*time.Second)) != nil || len(m.policyPulses) != 0 {
		t.Fatal("pulse never ended")
	}
	next := json.RawMessage(strings.Replace(string(data), `"turn":1`, `"turn":2`, 1))
	if m.detectPolicy(next) == nil {
		t.Fatal("next turn failed to pulse")
	}
	t.Setenv("NO_COLOR", "1")
	if m.pulseColor(key) != "" {
		t.Fatal("NO_COLOR ignored")
	}
}
func TestLoomPolicyAlias(t *testing.T) {
	m := policyFixture()
	m.focus = 3
	m.command.SetValue("/loom-control-policy")
	choices := m.commandChoices()
	if len(choices) == 0 || choices[0].id != "policy" {
		t.Fatal(choices)
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog == nil || m.dialog.kind != "policy" {
		t.Fatal("alias did not open config")
	}
}

func TestPulseBackoffAndCheckpointIdentity(t *testing.T) {
	for _, second := range []int{0, 2, 6, 14, 30} {
		if _, on := pulsePhase(time.Duration(second) * time.Second); !on {
			t.Fatal("missing pulse", second)
		}
	}
	for _, second := range []int{1, 3, 7, 15, 31} {
		if _, on := pulsePhase(time.Duration(second) * time.Second); on {
			t.Fatal("no backoff", second)
		}
	}
	m := policyFixture()
	m.detectPolicy(json.RawMessage(`{"run":"r","conversation":0,"turn":1,"id":"looping","check":0}`))
	first := m.policyPulses["r:0"].Started
	m.detectPolicy(json.RawMessage(`{"run":"r","conversation":0,"turn":1,"id":"looping","check":1}`))
	if m.policyPulses["r:0"].Started == first {
		t.Fatal("new checkpoint did not restart pulses")
	}
}
