package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestJudgeBundledRequiresConfirmation(t *testing.T) {
	m := evalFixture()
	m.openPolicyJudge("policy", 0)
	m.dialog.index = 1
	m.submitDialog()
	m.dialog.index = 1
	m.submitDialog()
	if m.dialog.kind != "eval-policy-judge-bundled" {
		t.Fatal("bundled skipped warning")
	}
	if m.data.EvaluationPolicies[0].Judges[0].CallMode == "bundled" {
		t.Fatal("mutated before confirmation")
	}
	m.dialog.index = 1
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var judges []evaluationJudge
	json.Unmarshal(req.Args["judges"], &judges)
	if judges[0].CallMode != "bundled" {
		t.Fatal("did not save confirmed mode")
	}
	if m.dialog.kind != "eval-policy-judge" {
		t.Fatal("must return to judge")
	}
}
func TestLegacyRunJudgeDisplayCompatibility(t *testing.T) {
	var run evaluationRun
	err := json.Unmarshal([]byte(`{"policy":{"judges":[{"id":"old","name":"Coherent","kind":"llm","model":"old-model","spec":"Clear prose","threshold":0.8}]}}`), &run)
	if err != nil || len(run.Policy.Judges) != 1 || len(run.Policy.Judges[0].Behaviors) != 1 || run.Policy.Judges[0].Behaviors[0].Spec != "Clear prose" {
		t.Fatal(run, err)
	}
}
func TestPolicyJudgeDirectNavigation(t *testing.T) {
	m := evalFixture()
	m.openEvaluationPolicy("policy")
	root := m.dialog
	m.dialog.index = 2
	m.submitDialog()
	if m.dialog.kind != "eval-policy-judge" {
		t.Fatal(m.dialog)
	}
	for _, r := range m.dialog.rows {
		if r.id == "name" || r.id == "library" || r.id == "new" || r.id == "all-judges" {
			t.Fatal(r)
		}
	}
	m.closeDialog()
	if m.dialog != root {
		t.Fatal("Escape should return to policy")
	}
}

func TestPolicyBehaviorEditableWithoutJudge(t *testing.T) {
	m := evalFixture()
	m.data.EvaluationPolicies[0].Judges = nil
	m.openPolicyBehavior("policy", 0)
	m.dialog.index = 1
	m.submitDialog()
	m.dialog.fields[0].input.SetValue("New name")
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var behaviors []evaluationBehavior
	json.Unmarshal(req.Args["behaviors"], &behaviors)
	if len(behaviors) != 1 || behaviors[0].Name != "New name" {
		t.Fatal(req)
	}
	if _, ok := m.dialog.args["judge"]; ok {
		t.Fatal("behavior depends on judge context")
	}
}
func TestLegacyMultipleJudgesRequireSingleModelChoice(t *testing.T) {
	m := evalFixture()
	m.data.EvaluationPolicies[0].Judges = append(m.data.EvaluationPolicies[0].Judges, evaluationJudge{ID: "old", Kind: "llm"})
	m.openEvaluationPolicy("policy")
	root := m.dialog
	m.dialog.index = 2
	m.submitDialog()
	if m.dialog.kind != "eval-policy-judge-model" {
		t.Fatal(m.dialog)
	}
	for _, r := range m.dialog.rows {
		if !strings.HasPrefix(r.id, "llm:") {
			t.Fatal("picker must contain only LLMs", r)
		}
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var judges []evaluationJudge
	json.Unmarshal(req.Args["judges"], &judges)
	if len(judges) != 1 || judges[0].Kind != "llm" {
		t.Fatal(judges)
	}
	var behaviors []evaluationBehavior
	json.Unmarshal(req.Args["behaviors"], &behaviors)
	if len(behaviors) != 1 || m.dialog != root {
		t.Fatal("preserve behaviors and return to policy")
	}
}

func TestPolicyBehaviorRemoveAndCancel(t *testing.T) {
	m := evalFixture()
	m.openPolicyBehaviors("policy")
	list := m.dialog
	m.submitDialog()
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	m.submitDialog()
	if m.dialog.kind != "eval-policy-behavior" || len(m.data.EvaluationPolicies[0].Behaviors) != 1 {
		t.Fatal("cancel removed behavior")
	}
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	m.dialog.index = 1
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var behaviors []evaluationBehavior
	json.Unmarshal(req.Args["behaviors"], &behaviors)
	if len(behaviors) != 0 || m.dialog != list || len(m.data.EvaluationPolicies[0].Behaviors) != 1 {
		t.Fatal("remove should send draft and return to policy behaviors")
	}
}
func TestHistoricalNestedAndFlatPolicyShapes(t *testing.T) {
	for _, input := range []string{
		`{"policy":{"judges":[{"id":"j","kind":"llm","behaviors":[{"id":"b","spec":"Old spec"}]}]}}`,
		`{"policy":{"judges":[{"id":"j","kind":"llm"}],"behaviors":[{"id":"b","spec":"New spec"}]}}`,
	} {
		var run evaluationRun
		if err := json.Unmarshal([]byte(input), &run); err != nil {
			t.Fatal(err)
		}
		if len(run.Policy.Judges) != 1 {
			t.Fatal(run)
		}
		if strings.Contains(input, "New spec") && (len(run.Policy.Behaviors) != 1 || len(run.Policy.Judges[0].Behaviors) != 0) {
			t.Fatal("flat history received fabricated nested behavior")
		}
	}
}

func TestJudgesDeleteEscapeReturnsPolicy(t *testing.T) {
	m := evalFixture()
	m.openEvaluationPolicy("policy")
	root := m.dialog
	choose := func(id string) {
		t.Helper()
		for n, r := range m.dialog.rows {
			if r.id == id {
				m.dialog.index = n
				m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
				return
			}
		}
		t.Fatal("missing", id)
	}
	choose("judges")
	if m.dialog.parent != root {
		t.Fatal("Judge must sit directly beneath policy")
	}
	choose("delete")
	m.dialog.index = 1
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter}) })
	json.Unmarshal(req.Args["judges"], &m.data.EvaluationPolicies[0].Judges)
	data, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: data})
	if m.dialog.kind != "eval-policy-config" {
		t.Fatal("escaped to deleted judge", m.dialog)
	}
	choose("name")
	m.dialog.fields[0].input.SetValue("Renamed policy")
	req = captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter}) })
	if string(req.Args["name"]) != `"Renamed policy"` {
		t.Fatal(req)
	}
}
func TestStaleJudgeDialogAndEditorCannotPanic(t *testing.T) {
	for _, field := range []string{"model", "call_mode"} {
		m := evalFixture()
		m.openPolicyJudge("policy", 0)
		for n, r := range m.dialog.rows {
			if r.id == field {
				m.dialog.index = n
			}
		}
		m.data.EvaluationPolicies[0].Judges = nil
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.dialog.kind != "eval-policy-judge-model" {
			t.Fatal("stale judge not recovered", field)
		}
	}
	m := evalFixture()
	m.openPolicyJudge("policy", 0)
	m.editReturn = m.dialog
	m.editing = "policy-judge-prompt"
	m.data.EvaluationPolicies[0].Judges = nil
	if cmd := m.savePolicyEditor("draft"); cmd != nil {
		t.Fatal("saved removed judge")
	}
}
