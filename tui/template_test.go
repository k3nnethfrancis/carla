package main

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestTemplateDraftShowsInputsWithoutSaving(t *testing.T) {
	m := selectionPromptFixture(t)
	previous, _ := m.currentJudgePrompt()
	if !strings.Contains(m.editor.Value(), "{{candidates}}") || m.pending {
		t.Fatal("missing explicit inputs or premature save")
	}
	m.cancelEdit()
	if m.selectionString("policy_prompt") != previous {
		t.Fatal("opening editor changed saved prompt")
	}
}
func TestTemplateHighlightPreservesViewportAndCursor(t *testing.T) {
	m := fixture()
	m.editing = "policy-judge-prompt"
	for _, width := range []int{30, 100} {
		m.editor.SetWidth(width)
		m.editor.SetHeight(8)
		m.editor.SetValue("漢字 {{behaviors.name}}\n{{text}}\nJSON {\"key\": true}")
		m.editor.Focus()
		m.editor.CursorEnd()
		before := m.editor.Value()
		rendered := m.templateEditorView()
		if ansi.Strip(rendered) != ansi.Strip(m.editor.View()) || m.editor.Value() != before {
			t.Fatal("highlight altered text or layout")
		}
		for _, line := range strings.Split(rendered, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("highlight overflow", line)
			}
		}
	}
}
func TestMonitoringOwnershipNavigationAndQuickDetection(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("monitoring")
	m.dialog.index = 1
	m.submitDialog()
	policy := m.dialog
	chooseBehaviorRow(t, m, "behaviors")
	list := m.dialog
	m.submitDialog()
	found := false
	for i, r := range m.dialog.rows {
		if r.id == "action" || r.id == "color" {
			t.Fatal("actions belong to policy", r)
		}
		if r.id == "decision" {
			m.dialog.index = i
			found = true
		}
	}
	if !found {
		t.Fatal("behavior needs detection rule")
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyRight}) })
	if req.Command != "operational.policy.save" {
		t.Fatal(req)
	}
	m.pending = false
	m.closeDialog()
	if m.dialog != list {
		t.Fatal("return to behaviors")
	}
	m.closeDialog()
	if m.dialog != policy {
		t.Fatal("return to policy")
	}
	chooseBehaviorRow(t, m, "judge")
	for _, r := range m.dialog.rows {
		if r.id == "detection" {
			t.Fatal("detection belongs to behavior")
		}
	}
	m.closeDialog()
	chooseBehaviorRow(t, m, "actions")
	m.submitDialog()
	if m.dialog.kind != "loom-policy-actions" {
		t.Fatal(m.dialog)
	}
}

func TestTemplateSaveErrorRemainsVisibleWithDraft(t *testing.T) {
	m := fixture()
	m.width, m.height = 60, 18
	m.editing = "policy-judge-prompt"
	m.editor.SetValue("{{unknown}}")
	m.status = "Error: Unknown template variable: {{unknown}}"
	m.reflow()
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, m.status) || m.editor.Value() != "{{unknown}}" {
		t.Fatal("validation error hidden or draft lost", view)
	}
}
