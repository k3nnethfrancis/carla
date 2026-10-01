package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestLongSpecScrollResizeAndSave(t *testing.T) {
	m := policyFixture()
	m.data.Workspace.Path = t.TempDir()
	m.width, m.height = 80, 24
	m.openBehaviors()
	chooseBehaviorRow(t, m, "new")
	chooseBehaviorRow(t, m, "spec")
	text := strings.Repeat("A long behavior rule with Unicode 日本語 and additional context.\n", 120) + "LAST RULE"
	m.Update(tea.PasteMsg{Content: text})
	if m.editor.Value() != text || m.editor.ScrollYOffset() == 0 {
		t.Fatal("long paste truncated or did not scroll")
	}
	end := m.editor.Line()
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.editor.Line() >= end {
		t.Fatal("wheel did not reach editor")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.editor.Line() >= end-3 {
		t.Fatal("page up did not navigate")
	}
	m.editor.MoveToEnd()
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if !strings.Contains(m.editor.View(), "LAST RULE") {
		t.Fatal("resize hid current cursor line")
	}
	if m.editor.Value() != text {
		t.Fatal("resize changed spec")
	}
	m.saveEditor()
	if m.behaviorDraft.Spec != text {
		t.Fatal("save truncated spec")
	}
}

func TestNewEvaluationUsesMultilineSpecEditor(t *testing.T) {
	m := evalFixture()
	m.width, m.height = 100, 30
	m.openPolicyBehaviors("policy", 0)
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	m.dialog.fields[0].input.SetValue("Long criteria")
	m.submitDialog()
	if m.dialog != nil || m.editing != "policy-behavior-new" || m.editor.Height() < 5 {
		t.Fatal("single-line creation spec")
	}
	spec := strings.Repeat("Criterion\n", 80)
	m.editor.SetValue(spec)
	req := captureCommand(t, m, m.saveEditor)
	var judges []evaluationJudge
	json.Unmarshal(req.Args["judges"], &judges)
	if req.Command != "evaluation.policy.save" || judges[0].Behaviors[len(judges[0].Behaviors)-1].Spec != spec {
		t.Fatal("criteria truncated")
	}
	data, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: data})
	if m.editing != "" || m.dialog == nil || m.dialog.kind != "eval-policy-behaviors" {
		t.Fatal("save did not return to judge behaviors")
	}
}
