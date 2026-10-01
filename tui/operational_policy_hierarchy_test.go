package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Both operational policies expose judges before behavior-specific controls.
func TestSelectionJudgeHierarchyAndEditorReturn(t *testing.T) {
	m := policyFixture()
	m.data.PolicyModel = "Local selector"
	m.data.PolicySpec = "Continue coherent candidates"
	m.openSelectionConfig()
	policy := m.dialog
	policy.index = 1
	m.submitDialog()
	judges := m.dialog
	if judges.title != "Selection judges" || !strings.Contains(judges.rows[0].label, "(LLM)") {
		t.Fatal(judges)
	}
	m.submitDialog()
	judge := m.dialog
	if judge.title != "Selection judge" || judge.parent != judges {
		t.Fatal(judge)
	}
	judge.index = 2
	m.submitDialog()
	behaviors := m.dialog
	m.submitDialog()
	behavior := m.dialog
	if behavior.title != "Selection behavior" || behavior.parent != behaviors {
		t.Fatal(behavior)
	}
	m.dialog.index = 1
	m.submitDialog()
	if m.editing != "policy_spec" || m.editReturn != behavior {
		t.Fatalf("editor lost behavior return: %s", m.editing)
	}
	m.cancelEdit()
	if m.dialog != behavior {
		t.Fatal("cancel skipped behavior")
	}
	for _, parent := range []*dialog{behaviors, judge, judges, policy} {
		m.closeDialog()
		if m.dialog != parent {
			t.Fatal("Escape skipped hierarchy")
		}
	}
}

func TestOperationalPolicyDescriptionsAndCompactRender(t *testing.T) {
	m := policyFixture()
	for _, open := range []func(){func() { m.openMonitorJudges() }, func() { m.openMonitorJudge() }, func() { m.openSelectionConfig() }, func() { m.openSelectionJudges() }, func() { m.openSelectionJudge() }, func() { m.openSelectionBehaviors() }, func() { m.openSelectionBehavior() }} {
		m.data.PolicySpec = "Coherent alternatives"
		open()
		for i, r := range m.dialog.rows {
			if r.preview == "" {
				t.Fatalf("missing description in %s: %s", m.dialog.title, r.id)
			}
			m.dialog.index = i
			for _, size := range [][2]int{{60, 18}, {120, 36}} {
				m.width, m.height = size[0], size[1]
				m.reflow()
				_ = m.View()
			}
		}
	}
}

func namedOperationalFixture() *model {
	m := policyFixture()
	m.data.OperationalPolicies = map[string][]operationalPolicy{
		"monitoring": {{ID: "active", Name: "Live", Config: map[string]any{"monitor_mode": "off"}}, {ID: "draft", Name: "Draft", Config: map[string]any{"monitor_mode": "diffusion", "monitor_call_mode": "separate", "monitor_dimensions": []map[string]any{{"id": "looping", "name": "Looping", "spec": "Repetition", "enabled": true, "action": "warn", "color": "amber", "decision": "most_likely", "threshold": .8}}}}},
		"selection":  {{ID: "selection-draft", Name: "Select coherent", Config: map[string]any{"selection_enabled": false, "model_alias": "policy-model", "policy_spec": "Original spec", "selection_behaviors": []map[string]any{{"id": "coherence", "name": "Coherence", "spec": "Original spec", "enabled": true}}}}},
	}
	m.data.ActiveOperationalPolicies = map[string]string{"monitoring": "active"}
	return m
}
func TestInactiveOperationalActionEditsNamedPolicyOnly(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("monitoring")
	m.dialog.index = 1
	m.submitDialog()
	if !strings.Contains(m.dialog.title, "Draft") {
		t.Fatal("missing policy name")
	}
	chooseBehaviorRow(t, m, "actions")
	m.submitDialog()
	if m.dialog.kind != "loom-policy-action" {
		t.Fatal(m.dialog)
	}
	chooseBehaviorRow(t, m, "action")
	m.dialog.index = 1
	req := captureCommand(t, m, m.submitDialog)
	if req.Command != "operational.policy.save" || string(req.Args["id"]) != `"draft"` {
		t.Fatal(req)
	}
	if m.data.ActiveOperationalPolicies["monitoring"] != "active" || m.data.SimulatorConfig["monitor_mode"] != "jev" {
		t.Fatal("editing mutated active runtime")
	}
	var c map[string]any
	json.Unmarshal(req.Args["config"], &c)
	items := c["monitor_dimensions"].([]any)
	if items[0].(map[string]any)["action"] != "stop" {
		t.Fatal(c)
	}
}
func TestNamedSelectionBehaviorEditorTargetsPolicyAndBehavior(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("selection")
	m.submitDialog()
	chooseBehaviorRow(t, m, "judges")
	m.submitDialog()
	chooseBehaviorRow(t, m, "behaviors")
	m.submitDialog()
	chooseBehaviorRow(t, m, "spec")
	if m.editor.Value() != "Original spec" {
		t.Fatal(m.editor.Value())
	}
	m.editor.SetValue("Updated coherent candidates")
	req := captureCommand(t, m, m.saveEditor)
	if req.Command != "operational.policy.save" || string(req.Args["id"]) != `"selection-draft"` {
		t.Fatal(req)
	}
	var c map[string]any
	json.Unmarshal(req.Args["config"], &c)
	b := c["selection_behaviors"].([]any)[0].(map[string]any)
	if b["id"] != "coherence" || b["spec"] != "Updated coherent candidates" {
		t.Fatal(c)
	}
}
func TestNamedMonitoringBehaviorHasNoPolicyActions(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("monitoring")
	m.dialog.index = 1
	m.submitDialog()
	chooseBehaviorRow(t, m, "judges")
	m.submitDialog()
	chooseBehaviorRow(t, m, "behaviors")
	m.submitDialog()
	for _, r := range m.dialog.rows {
		if r.id == "action" || r.id == "color" {
			t.Fatal("policy action leaked into behavior", r)
		}
	}
}

func TestNamedSelectionNewBehaviorWaitsForFullSpec(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("selection")
	m.submitDialog()
	chooseBehaviorRow(t, m, "judges")
	m.submitDialog()
	chooseBehaviorRow(t, m, "behaviors")
	list := m.dialog
	chooseBehaviorRow(t, m, "new-behavior")
	m.dialog.fields[0].input.SetValue("Voice")
	m.submitDialog()
	if m.editing != "operational-selection-new" || m.pending {
		t.Fatal("created before complete spec")
	}
	m.editor.SetValue("Stay consistent across the candidate\nIgnore quoted text.")
	req := captureCommand(t, m, m.saveEditor)
	var c map[string]any
	json.Unmarshal(req.Args["config"], &c)
	bs := c["selection_behaviors"].([]any)
	added := bs[len(bs)-1].(map[string]any)
	if added["name"] != "Voice" || added["spec"] != m.editor.Value() || added["id"] != nil {
		t.Fatal(added)
	}
	m.apply(stateEvent(t, m, req.ID))
	if m.editing != "" || m.dialog == nil || m.dialog.title != list.title {
		t.Fatal("successful save did not return to behaviors")
	}
}
func TestMonitoringOffKeepsConfiguredJudgeEditable(t *testing.T) {
	m := namedOperationalFixture()
	m.data.OperationalPolicies["monitoring"][1].Config["monitor_mode"] = "off"
	m.data.OperationalPolicies["monitoring"][1].Config["monitor_provider"] = "diffusion"
	m.openOperationalPolicies("monitoring")
	m.dialog.index = 1
	m.submitDialog()
	chooseBehaviorRow(t, m, "judges")
	m.submitDialog()
	if !strings.Contains(m.dialog.title, "DiffusionGemma") {
		t.Fatal(m.dialog.title)
	}
	if m.monitorString("monitor_mode") != "off" || m.pending {
		t.Fatal("inspection enabled monitoring")
	}
}

func TestDisabledMonitoringModelChangeStaysDisabled(t *testing.T) {
	m := namedOperationalFixture()
	c := m.data.OperationalPolicies["monitoring"][1].Config
	c["monitor_mode"], c["monitor_provider"] = "off", "diffusion"
	m.openOperationalPolicies("monitoring")
	m.dialog.index = 1
	m.submitDialog()
	chooseBehaviorRow(t, m, "judges")
	m.submitDialog()
	chooseBehaviorRow(t, m, "mode")
	m.dialog.index = 1
	req := captureCommand(t, m, m.submitDialog)
	var config map[string]any
	json.Unmarshal(req.Args["config"], &config)
	if config["monitor_mode"] != "off" || config["monitor_provider"] != "jev" {
		t.Fatal(config)
	}
}
func TestLegacyPolicyAliasesOpenNamedLists(t *testing.T) {
	for _, alias := range []string{"spec", "grow-policy", "loom-policy"} {
		m := namedOperationalFixture()
		m.perform(alias)
		if m.dialog == nil || m.dialog.kind != "operational-policy-list" || m.editing != "" {
			t.Fatal(alias, m.dialog, m.editing)
		}
	}
}
