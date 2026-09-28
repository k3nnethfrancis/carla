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
	m := policyFixture()
	m.width, m.height = 100, 30
	parent := &dialog{kind: "eval-definitions"}
	d := &dialog{kind: "eval-new", parent: parent, args: map[string]any{"kind": "jev"}}
	d.add("Name", "Long criteria")
	m.dialog = d
	m.submitDialog()
	if m.dialog != nil || m.editing != "evaluation-new-spec" || m.editor.Height() < 5 {
		t.Fatal("single-line creation spec")
	}
	spec := strings.Repeat("Criterion\n", 80)
	m.editor.SetValue(spec)
	req := captureCommand(t, m, m.saveEditor)
	var actual string
	json.Unmarshal(req.Args["spec"], &actual)
	if req.Command != "evaluation.configure" || actual != spec {
		t.Fatal("criteria truncated")
	}
	data, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: data})
	if m.editing != "" || m.dialog == nil || m.dialog.kind != "eval-definitions" {
		t.Fatal("save did not return to evaluations")
	}
}
