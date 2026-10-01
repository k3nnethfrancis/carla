package main

import (
	tea "charm.land/bubbletea/v2"
	"strings"
	"testing"
)

func chooseBehaviorRow(t *testing.T, m *model, id string) tea.Cmd {
	t.Helper()
	for i, r := range m.dialog.rows {
		if r.id == id {
			m.dialog.index = i
			return m.submitDialog()
		}
	}
	t.Fatal("missing row", id)
	return nil
}

func TestBehaviorDraftFullConfigurationAndMultilineSpec(t *testing.T) {
	m := policyFixture()
	m.width, m.height = 120, 36
	m.openBehaviors()
	chooseBehaviorRow(t, m, "new")
	for _, id := range []string{"enabled", "name", "spec", "decision", "create"} {
		found := false
		for _, r := range m.dialog.rows {
			if r.id == id {
				found = true
			}
		}
		if !found {
			t.Fatal("missing draft setting", id)
		}
	}
	chooseBehaviorRow(t, m, "name")
	m.dialog.fields[0].input.SetValue("Voice drift")
	m.submitDialog()
	chooseBehaviorRow(t, m, "enabled")
	if m.behaviorDraft.Enabled {
		t.Fatal("draft toggle not retained")
	}
	chooseBehaviorRow(t, m, "spec")
	if m.dialog != nil || m.editing != "monitor_spec" || m.editor.Height() < 5 {
		t.Fatal("spec not in full editor")
	}
	spec := "Flag loss of voice.\n\nIgnore deliberate quotations."
	m.editor.SetValue(spec)
	m.saveEditor()
	if m.behaviorDraft.Spec != spec || m.dialog == nil || m.editing != "" {
		t.Fatal("multiline draft not preserved")
	}
	chooseBehaviorRow(t, m, "decision")
	m.dialog.index = 1
	m.submitDialog()
	chooseBehaviorRow(t, m, "threshold")
	m.dialog.fields[0].input.SetValue("90")
	m.submitDialog()
	if m.behaviorDraft.Threshold != .9 {
		t.Fatal("threshold not saved locally")
	}
	req := captureCommand(t, m, func() tea.Cmd { return chooseBehaviorRow(t, m, "create") })
	if req.Command != "loom-policy.add" || string(req.Args["enabled"]) != "false" || string(req.Args["decision"]) != `"threshold"` || string(req.Args["threshold"]) != "0.9" {
		t.Fatal(req)
	}
}

func TestBehaviorCancelAndListStatus(t *testing.T) {
	m := policyFixture()
	m.openBehaviors()
	if !strings.Contains(m.dialog.rows[1].label, "Disabled") {
		t.Fatal("list hides enabled state")
	}
	chooseBehaviorRow(t, m, "new")
	if cmd := chooseBehaviorRow(t, m, "create"); cmd != nil {
		t.Fatal("empty behavior submitted")
	}
	chooseBehaviorRow(t, m, "spec")
	m.editor.SetValue("discard me")
	m.cancelEdit()
	if m.behaviorDraft.Spec != "" {
		t.Fatal("cancel saved text")
	}
	m.closeDialog()
	if m.behaviorDraft != nil || m.dialog.kind != "loom-policy-behaviors" {
		t.Fatal("draft cancel lost parent or retained draft")
	}
}
