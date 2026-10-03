package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func chooseRunField(t *testing.T, m *model, id string) {
	t.Helper()
	for i, r := range m.dialog.rows {
		if r.id == id {
			m.dialog.index = i
			m.submitDialog()
			return
		}
	}
	t.Fatal("missing field", id)
}

func TestNewRunDraftScopeDefaultsAndRecovery(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalArea = "data"
	m.evalCollection = "set"
	m.data.EvaluationSets[0].Items = []evaluationSummary{{ID: "a", Status: "complete"}, {ID: "b", Status: "unjudged"}}
	m.data.EvaluationSets = append(m.data.EvaluationSets, evaluationCollection{ID: "second", Name: "Other data", Items: []evaluationSummary{{ID: "c"}}})
	m.data.EvaluationPolicies = append(m.data.EvaluationPolicies, evaluationPolicy{ID: "second-policy", Name: "Other policy", Actions: evaluationActions{TrainOnPass: true}})
	m.evalSelection = map[string]bool{"a": true}
	original, _ := json.Marshal(m.data)
	if cmd := m.openEval(`/eval "Other policy" --train-on-pass false`); cmd != nil {
		t.Fatal("setup started inference")
	}
	draft := m.dialog.args["draft"].(*evaluationRunDraft)
	if draft.Dataset != "set" || draft.Policy != "second-policy" || !reflect.DeepEqual(draft.Selected, []string{"a"}) || !draft.UseSelected || draft.TrainOnPass == nil || *draft.TrainOnPass {
		t.Fatal(draft)
	}
	parent := m.dialog
	chooseRunField(t, m, "dataset")
	m.closeDialog()
	if m.dialog != parent || draft.Dataset != "set" {
		t.Fatal("Escape changed dataset or lost draft")
	}
	chooseRunField(t, m, "dataset")
	chooseRunField(t, m, "second")
	if draft.Dataset != "second" || draft.UseSelected || len(draft.Selected) != 0 {
		t.Fatal("selection leaked across datasets", draft)
	}
	chooseRunField(t, m, "training")
	chooseRunField(t, m, "default")
	if draft.TrainOnPass != nil || !strings.Contains(m.dialog.rows[3].label, "Mark for training") {
		t.Fatal("policy default not reflected")
	}
	after, _ := json.Marshal(m.data)
	if string(original) != string(after) {
		t.Fatal("setup mutated saved settings")
	}
	m.dialog.index = 4
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	if req.Command != "evaluation.collection.run" || string(req.Args["items"]) != `["c"]` || string(req.Args["collection"]) != `"second"` || string(req.Args["policy"]) != `"second-policy"` {
		t.Fatal(req)
	}
	if _, ok := req.Args["train_on_pass"]; ok {
		t.Fatal("overrode policy default")
	}
	m.apply(event{Type: "error", ID: req.ID, Data: json.RawMessage(`{"message":"No local judge configured"}`)})
	if m.dialog == nil || m.dialog.args["draft"] != draft || m.dialog.args["error"] == nil {
		t.Fatal("failed run lost setup")
	}
	req = captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	raw, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: raw})
	if m.dialog != nil || m.evalArea != "runs" || m.evalCollection != "" {
		t.Fatal("accepted run did not open Results")
	}
}

func TestNewRunTargetsFocusedDatasetAndIncludesCompletedItems(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalArea = "data"
	m.focus = 3
	m.data.EvaluationSets = append(m.data.EvaluationSets, evaluationCollection{ID: "second", Name: "Other", Items: []evaluationSummary{{ID: "done", Status: "complete"}}})
	m.selected = 3
	m.command.SetValue("/eval")
	if choices := m.commandChoices(); len(choices) == 0 || choices[0].id != "eval" {
		t.Fatal("dataset has no eval command", choices)
	}
	m.openEval("/eval")
	draft := m.dialog.args["draft"].(*evaluationRunDraft)
	if draft.Dataset != "second" || draft.UseSelected {
		t.Fatal(draft)
	}
	m.dialog.index = 4
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	if string(req.Args["items"]) != `["done"]` {
		t.Fatal("silently skipped completed data", req)
	}
}

func TestNewRunMissingDataAndPolicyStayInSetup(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.data.EvaluationSets = nil
	m.data.ActiveEvaluation = ""
	m.data.EvaluationPolicies = nil
	m.data.ActiveEvaluationPolicy = ""
	m.evaluationCollectionAction("eval-new-run", "")
	draft := m.dialog.args["draft"]
	for _, field := range []string{"dataset", "policy", "start"} {
		chooseRunField(t, m, field)
		if m.dialog.kind != "eval-run-config" || m.dialog.args["error"] == nil || m.dialog.args["draft"] != draft {
			t.Fatal("missing setup context", m.dialog)
		}
	}
	m.closeDialog()
	if m.dialog != nil {
		t.Fatal("cancel failed")
	}
}

func TestEvaluatePoliciesUsesSameEntryPoint(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evaluationCollectionAction("eval-area", "policies")
	fromPage := m.dialog
	m.openPolicy()
	m.dialog.index = 2
	m.submitDialog()
	if fromPage.kind != m.dialog.kind || !reflect.DeepEqual(fromPage.rows, m.dialog.rows) {
		t.Fatal("two policy interfaces")
	}
	if evaluationAreaTitle("runs") != "Runs" {
		t.Fatal("old saved route not renamed")
	}
}
