package main

import (
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
	if behavior.title != "Selection criteria" || behavior.parent != behaviors {
		t.Fatal(behavior)
	}
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
