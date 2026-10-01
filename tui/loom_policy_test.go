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
	m.data.MonitorKeySource = "environment"
	m.data.SimulatorConfig = map[string]any{"monitor_mode": "jev", "monitor_model": "jev-latest", "monitor_dimensions": []any{
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
	m.submitDialog()
	m.dialog.index = 2
	m.submitDialog()
	if m.dialog.kind != "loom-policy-behaviors" {
		t.Fatal("missing behaviors page")
	}
	behaviors := m.dialog
	m.submitDialog()
	if m.dialog.kind != "loom-policy-dimension" || m.dialog.parent != behaviors {
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
	if m.dialog.kind != "loom-policy-behaviors" {
		t.Fatal("Escape skipped behaviors")
	}
	m.dialog.index = 1
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
	m.openMonitorJudge()
	m.dialog.index = 0
	m.submitDialog()
	if m.dialog.kind != "loom-policy-pick" {
		t.Fatal("model picker did not open")
	}
	m.closeDialog()
	m.dialog.index = 2
	m.submitDialog()
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	if m.behaviorDraft == nil || m.dialog.kind != "loom-policy-dimension" {
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

func TestMonitorTimingNavigationAndSavedInterval(t *testing.T) {
	m := policyFixture()
	m.width, m.height = 120, 36
	m.data.SimulatorConfig["monitor_interval_tokens"] = float64(768)
	m.openLoomPolicy()
	root := m.dialog
	root.index = 1
	m.submitDialog()
	timing := m.dialog
	if timing.kind != "loom-policy-timing" || timing.parent != root || len(timing.rows) != 3 {
		t.Fatal("missing timing page")
	}
	timing.index = 2
	m.submitDialog()
	if m.dialog.kind != "config-number" || m.dialog.parent != timing || m.dialog.fields[0].input.Value() != "768" {
		t.Fatal("interval did not open")
	}
	m.closeDialog()
	if m.dialog != timing {
		t.Fatal("Escape skipped timing page")
	}
	m.data.SimulatorConfig["monitor_during_reply"] = false
	m.refreshConfig()
	if len(m.dialog.rows) != 2 || m.monitorInterval() != 768 {
		t.Fatal("off discarded interval")
	}
	m.data.SimulatorConfig["monitor_during_reply"] = true
	m.refreshConfig()
	if len(m.dialog.rows) != 3 || !strings.Contains(m.dialog.rows[2].label, "768") {
		t.Fatal("interval not restored")
	}
	m.closeDialog()
	if m.dialog.kind != "loom-policy" {
		t.Fatal("Escape skipped monitoring policy")
	}
	m.data.SimulatorConfig["monitor_interval_tokens"] = float64(0)
	m.openMonitorTiming()
	if !strings.Contains(m.dialog.rows[1].label, "Off") || len(m.dialog.rows) != 2 {
		t.Fatal("legacy end-only not honored")
	}
}

func TestMonitoringSetupGatesSettingsAndMasksKey(t *testing.T) {
	m := policyFixture()
	m.width, m.height = 100, 30
	m.data.SimulatorConfig["monitor_mode"] = "off"
	m.data.MonitorKeySource = ""
	m.openLoomPolicy()
	root := m.dialog
	if len(root.rows) != 1 {
		t.Fatal("Off exposed monitoring configuration")
	}
	m.submitDialog() // choose a provider
	m.dialog.index = 1
	m.submitDialog() // Jev needs a key first
	if m.dialog.kind != "loom-policy-key" || m.simString("monitor_mode") != "off" {
		t.Fatal("enabled before key setup")
	}
	m.dialog.fields[0].input.SetValue("secret-must-not-render")
	if strings.Contains(m.View().Content, "secret-must-not-render") {
		t.Fatal("key exposed in terminal")
	}
	m.closeDialog()
	if m.dialog != root || m.simString("monitor_mode") != "off" {
		t.Fatal("cancel changed configuration")
	}
	m.data.SimulatorConfig["monitor_mode"] = "jev"
	m.openLoomPolicy()
	if len(m.dialog.rows) != 2 || m.dialog.rows[1].id != "key" {
		t.Fatal("legacy Jev without key exposed options")
	}
	m.data.MonitorKeySource = "environment"
	m.openLoomPolicy()
	if len(m.dialog.rows) != 3 || m.dialog.rows[2].id != "judges" {
		t.Fatal("environment key did not unlock settings")
	}
}

func TestLocalMonitorHasNoKeyGateAndRetainsNavigation(t *testing.T) {
	m := policyFixture()
	m.data.MonitorKeySource = ""
	m.data.SimulatorConfig["monitor_mode"] = "diffusion"
	m.data.SimulatorConfig["monitor_local_url"] = "http://127.0.0.1:8080"
	m.data.SimulatorConfig["monitor_local_model"] = "openjev-latest"
	m.openLoomPolicy()
	root := m.dialog
	for _, r := range root.rows {
		if r.id == "key" || r.id == "monitor_local_url" {
			t.Fatal("local mode requested infrastructure configuration")
		}
	}
	if !strings.Contains(root.rows[0].label, "On") {
		t.Fatal(root.rows)
	}
	root.index = 2
	m.submitDialog()
	judges := m.dialog
	m.submitDialog()
	if m.dialog.kind != "loom-policy-judge" || m.dialog.parent != judges {
		t.Fatal("judge did not retain hierarchy")
	}
	m.closeDialog()
	m.closeDialog()
	if m.dialog != root {
		t.Fatal("Escape did not return to policy")
	}
	m.openMonitorTiming()
	if strings.Contains(m.dialog.title, "off") {
		t.Fatal("local heartbeat labeled off")
	}
	for _, size := range [][2]int{{60, 18}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		m.reflow()
		_ = m.View()
	}
}

func TestLocalModelSavePreservesRejectedDraftUntilAcknowledged(t *testing.T) {
	m := policyFixture()
	m.width, m.height = 60, 18
	m.openLoomPolicy()
	parent := m.dialog
	m.policyForm(parent, "", "monitor_local_model", "")
	draft := m.dialog
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	if m.dialog != draft {
		t.Fatal("closed before validation")
	}
	m.apply(event{Type: "error", ID: req.ID, Data: json.RawMessage(`{"message":"Local judge model must not be empty"}`)})
	if m.dialog != draft || draft.fields[0].input.Value() != "" || !strings.Contains(m.View().Content, "must not be empty") {
		t.Fatal("lost validation or draft")
	}
	draft.fields[0].input.SetValue("openjev-latest")
	req = captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	m.apply(stateEvent(t, m, req.ID))
	if m.dialog.kind != "loom-policy" || m.dialogRequest != "" {
		t.Fatal("successful save did not go back")
	}
}

func TestMonitoringLayoutAndBehaviorCounts(t *testing.T) {
	m := policyFixture()
	m.data.SimulatorConfig["monitor_mode"] = "diffusion"
	m.data.SimulatorConfig["monitor_local_model"] = "openjev-latest"
	m.data.SimulatorConfig["monitor_dimensions"] = []loomDimension{
		{Enabled: true, Action: "warn"}, {Enabled: true, Action: "warn"},
		{Enabled: true, Action: "stop"}, {Enabled: false, Action: "stop"},
	}
	m.openLoomPolicy()
	for i, id := range []string{"mode", "timing", "judges"} {
		if m.dialog.rows[i].id != id {
			t.Fatal(m.dialog.rows)
		}
	}
	m.openMonitorJudge()
	if m.dialog.rows[2].label != "Behaviors · 2 warn · 1 stop · 1 off" {
		t.Fatal(m.dialog.rows[2].label)
	}
	if !strings.Contains(m.dialog.rows[0].label, "DiffusionGemma (classifier)") {
		t.Fatal("missing model name and type")
	}
}
