package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"testing"
)

func TestPolicyHubConsistentAcrossTabs(t *testing.T) {
	for tab := 0; tab < 5; tab++ {
		m := evalFixture()
		m.section = tab
		m.openPolicy()
		if m.dialog.kind != "policy" || len(m.dialog.rows) != 3 || m.dialog.rows[2].label != "Evals" {
			t.Fatal(tab, m.dialog)
		}
		m.dialog.index = 2
		m.submitDialog()
		if m.dialog.kind != "eval-policy-list" || m.dialog.parent.kind != "policy" {
			t.Fatal("eval link", tab)
		}
	}
}
func TestImportBehaviorLibraryKeepsLocalSettings(t *testing.T) {
	m := evalFixture()
	m.data.BehaviorLibrary = []libraryBehavior{{ID: "lib", Name: "Voice", Spec: "Stable voice", Revision: 3}}
	m.openPolicyBehaviors("policy")
	m.dialog.index = len(m.dialog.rows) - 2
	m.submitDialog()
	if m.dialog.kind != "eval-policy-behavior-library" {
		t.Fatal(m.dialog)
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var behaviors []evaluationBehavior
	json.Unmarshal(req.Args["behaviors"], &behaviors)
	b := behaviors[len(behaviors)-1]
	if b.SourceID != "lib" || b.SourceRevision != 3 || b.Spec != "Stable voice" || b.Enabled || b.Expected != "present" || b.Threshold != .8 {
		t.Fatal(b)
	}
	if m.dialog.kind != "eval-policy-behaviors" {
		t.Fatal("return to behaviors")
	}
}
func TestLibraryCreateUsesMultilineAndCorrelatedSave(t *testing.T) {
	m := evalFixture()
	m.openBehaviorLibrary()
	m.submitDialog()
	m.dialog.fields[0].input.SetValue("Voice")
	m.submitDialog()
	m.editor.SetValue("First line\nSecond line")
	req := captureCommand(t, m, m.saveEditor)
	if req.Command != "behavior.save" {
		t.Fatal(req)
	}
	if m.editReturn.kind != "behavior-library-new" {
		t.Fatal("draft discarded before save confirmation")
	}
	data, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: data})
	if m.dialog.kind != "behavior-library" {
		t.Fatal("wrong return")
	}
}
func TestEvalActionDefaultAndExplicitOverride(t *testing.T) {
	m := evalFixture()
	m.data.EvaluationPolicies[0].Actions.TrainOnPass = true
	m.openEvaluationPolicy("policy")
	m.dialog.index = 3
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var actions evaluationActions
	json.Unmarshal(req.Args["actions"], &actions)
	if actions.TrainOnPass {
		t.Fatal("toggle did not save")
	}
	for _, input := range []string{"/eval", "/eval --train-on-pass false"} {
		m := evalFixture()
		m.section = 1
		m.branchSelection = map[string]bool{m.currentID(): true}
		req := captureCommand(t, m, func() tea.Cmd { return m.openEval(input) })
		_, has := req.Args["train_on_pass"]
		if has != (input != "/eval") {
			t.Fatal("default must be omitted", req)
		}
	}
}
func TestBehaviorsCommandIsPrimary(t *testing.T) {
	m := evalFixture()
	if !m.paletteAvailable("behaviors", "/") {
		t.Fatal("library hidden")
	}
	m.perform("behaviors")
	if m.dialog.kind != "behavior-library" {
		t.Fatal("command did not open library")
	}
}
