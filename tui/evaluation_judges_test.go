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
	m.dialog.index = 2
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
func TestNewJudgeThenBehaviorNavigation(t *testing.T) {
	m := evalFixture()
	m.openPolicyJudges("policy")
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	if m.dialog.kind != "eval-policy-judge-model" {
		t.Fatal(m.dialog)
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var judges []evaluationJudge
	json.Unmarshal(req.Args["judges"], &judges)
	j := judges[len(judges)-1]
	if j.Kind != "diffusion" || j.CallMode != "separate" {
		t.Fatal(j)
	}
	if m.dialog.kind != "eval-policy-judges" {
		t.Fatal("new judge returns to list")
	}
}

func TestJudgeBackRefreshesParentCounts(t *testing.T) {
	m := evalFixture()
	m.openPolicyJudges("policy")
	parent := m.dialog
	m.openPolicyJudge("policy", 0)
	m.dialog.parent = parent
	m.data.EvaluationPolicies[0].Behaviors = append(m.data.EvaluationPolicies[0].Behaviors, evaluationBehavior{Name: "Second", Enabled: true})
	m.closeDialog()
	if m.dialog.kind != "eval-policy-judges" || !strings.Contains(m.dialog.rows[0].preview, "2 behaviors") {
		t.Fatalf("stale parent after child edit: %+v", m.dialog)
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
func TestMultipleJudgesPreserveOneBehaviorSet(t *testing.T) {
	m := evalFixture()
	m.data.EvaluationPolicies[0].Judges = append(m.data.EvaluationPolicies[0].Judges, evaluationJudge{ID: "two", Name: "Second", Kind: "diffusion", Model: "openjev-latest"})
	m.openEvaluationPolicy("policy")
	m.dialog.index = 2
	m.submitDialog()
	if m.dialog.kind != "eval-policy-judges" {
		t.Fatal("multiple judges should list models")
	}
	m.dialog.index = 1
	m.submitDialog()
	m.dialog.index = 0
	m.submitDialog()
	m.dialog.fields[0].input.SetValue("Renamed second")
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var behaviors []evaluationBehavior
	json.Unmarshal(req.Args["behaviors"], &behaviors)
	var judges []map[string]any
	json.Unmarshal(req.Args["judges"], &judges)
	if len(behaviors) != 1 || len(judges) != 2 || judges[1]["name"] != "Renamed second" {
		t.Fatal(req)
	}
	for _, j := range judges {
		if _, ok := j["behaviors"]; ok {
			t.Fatal("live judges must not serialize nested behaviors")
		}
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
		t.Fatal("Judges list must sit directly beneath policy")
	}
	choose("0")
	choose("delete")
	m.dialog.index = 1
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter}) })
	json.Unmarshal(req.Args["judges"], &m.data.EvaluationPolicies[0].Judges)
	data, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: data})
	if m.dialog.kind != "eval-policy-judges" {
		t.Fatal(m.dialog)
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
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
	for _, field := range []string{"name", "model", "call_mode"} {
		m := evalFixture()
		m.openPolicyJudge("policy", 0)
		for n, r := range m.dialog.rows {
			if r.id == field {
				m.dialog.index = n
			}
		}
		m.data.EvaluationPolicies[0].Judges = nil
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.dialog.kind != "eval-policy-judges" {
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

func TestJudgeLibraryCopiesConfigurationWithoutBehaviorsOrIdentity(t *testing.T) {
	m := evalFixture()
	source := evaluationPolicy{ID: "source", Name: "Other policy", Judges: []evaluationJudge{{ID: "original", Revision: 7, Name: "Careful reader", Kind: "llm", Model: "other-model", Prompt: "Custom {{behaviors}} {{text}}", CallMode: "bundled"}}, Behaviors: []evaluationBehavior{{ID: "source-only", Name: "Other criteria"}}}
	m.data.EvaluationPolicies = append(m.data.EvaluationPolicies, source)
	original, _ := json.Marshal(m.data.EvaluationPolicies)
	m.openPolicyJudges("policy")
	parent := m.dialog
	for i, r := range parent.rows {
		if r.id == "library" {
			parent.index = i
		}
	}
	m.submitDialog()
	if m.dialog.kind != "eval-policy-judge-library" || m.dialog.parent != parent {
		t.Fatal("library belongs beside New judge")
	}
	m.closeDialog()
	if m.dialog != parent {
		t.Fatal("Escape must return to Judges")
	}
	m.openJudgeLibraryPicker(parent)
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var judges []evaluationJudge
	json.Unmarshal(req.Args["judges"], &judges)
	copy := judges[len(judges)-1]
	if copy.ID != "" || copy.Revision != 0 || copy.Model != "other-model" || copy.Prompt != source.Judges[0].Prompt || copy.CallMode != "bundled" {
		t.Fatal("copy lost settings or retained identity", copy)
	}
	var behaviors []evaluationBehavior
	json.Unmarshal(req.Args["behaviors"], &behaviors)
	if len(behaviors) != len(m.data.EvaluationPolicies[0].Behaviors) || behaviors[0].ID == "source-only" {
		t.Fatal("import changed policy behaviors")
	}
	after, _ := json.Marshal(m.data.EvaluationPolicies)
	if string(original) != string(after) {
		t.Fatal("picker mutated saved policies before backend validation")
	}
	if m.dialog != parent {
		t.Fatal("import must return to judge list")
	}
	m.pending = false
	m.openPolicyJudge("policy", 0)
	for _, r := range m.dialog.rows {
		if r.id == "all-judges" {
			t.Fatal("redundant shortcut remains")
		}
	}
}
