package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestEvaluationExpectedQuickKeysPreservePolicy(t *testing.T) {
	for _, key := range []rune{tea.KeySpace, tea.KeyLeft, tea.KeyRight, tea.KeyEnter} {
		m := evalFixture()
		m.data.EvaluationPolicies[0].Actions.TrainOnPass = true
		m.openPolicyBehavior("policy", 0)
		for n, r := range m.dialog.rows {
			if r.id == "expected" {
				m.dialog.index = n
			}
		}
		req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: key}) })
		var b []evaluationBehavior
		json.Unmarshal(req.Args["behaviors"], &b)
		var actions evaluationActions
		json.Unmarshal(req.Args["actions"], &actions)
		if b[0].Expected != "absent" || !b[0].Enabled || !actions.TrainOnPass {
			t.Fatal(req)
		}
		if m.dialog.kind != "eval-policy-behavior" || m.data.EvaluationPolicies[0].Behaviors[0].Expected != "" {
			t.Fatal("optimistic mutation or wrong return")
		}
	}
}
func TestEvaluationOnPassDirectQuickKeys(t *testing.T) {
	for _, key := range []rune{tea.KeySpace, tea.KeyLeft, tea.KeyRight, tea.KeyEnter} {
		m := evalFixture()
		m.openEvaluationPolicy("policy")
		for n, r := range m.dialog.rows {
			if r.id == "on-pass" {
				m.dialog.index = n
				if r.label != "On pass · Record only" {
					t.Fatal(r)
				}
			}
			if r.id == "actions" {
				t.Fatal("redundant actions submenu")
			}
		}
		req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: key}) })
		var actions evaluationActions
		json.Unmarshal(req.Args["actions"], &actions)
		if !actions.TrainOnPass || m.dialog.kind != "eval-policy-config" {
			t.Fatal(req, m.dialog)
		}
	}
}
func TestSelectionExpectedAndLibraryReview(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("selection")
	m.submitDialog()
	chooseBehaviorRow(t, m, "behaviors")
	chooseBehaviorRow(t, m, "coherence")
	for n, r := range m.dialog.rows {
		if r.id == "behavior-expected" {
			m.dialog.index = n
		}
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyRight}) })
	var config struct {
		Behaviors []map[string]any `json:"selection_behaviors"`
	}
	json.Unmarshal(req.Args["config"], &config)
	if len(config.Behaviors) != 1 || config.Behaviors[0]["expected"] != "absent" || string(req.Args["id"]) != `"selection-draft"` {
		t.Fatal(req)
	}
	m = namedOperationalFixture()
	m.data.BehaviorLibrary = []libraryBehavior{{ID: "harm", Name: "Harm", Spec: "Judge latest message", Revision: 2}}
	m.openOperationalPolicies("selection")
	m.submitDialog()
	chooseBehaviorRow(t, m, "behaviors")
	chooseBehaviorRow(t, m, "library-behavior")
	if !strings.Contains(m.dialog.rows[0].preview, "Imports Off") {
		t.Fatal(m.dialog)
	}
	req = captureCommand(t, m, m.submitDialog)
	json.Unmarshal(req.Args["config"], &config)
	b := config.Behaviors[len(config.Behaviors)-1]
	if b["enabled"] != false || b["spec"] != "Judge latest message" || b["expected"] != "present" {
		t.Fatal(b)
	}
}
func TestSelectionAssessmentTemplateConfirmCancelAndNamedSave(t *testing.T) {
	m := namedOperationalFixture()
	m.data.OperationalPolicies["selection"][0].Config["selection_assessment_prompt"] = "Assess presence"
	m.openOperationalPolicies("selection")
	m.submitDialog()
	chooseBehaviorRow(t, m, "judge")
	chooseBehaviorRow(t, m, "assessment-prompt")
	if m.editing != "selection_assessment_prompt" || m.dialog != nil || m.editor.Value() != explicitTemplate("selection_assessment_prompt", "Assess presence") {
		t.Fatal("wrong editor")
	}
	m.editor.SetValue("Changed\nContract")
	m.saveEditor()
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != nil || m.editor.Value() != "Changed\nContract" {
		t.Fatal("lost draft")
	}
	m.saveEditor()
	m.dialog.index = 1
	req := captureCommand(t, m, m.submitDialog)
	var config map[string]any
	json.Unmarshal(req.Args["config"], &config)
	if req.Command != "operational.policy.save" || string(req.Args["id"]) != `"selection-draft"` || config["selection_assessment_prompt"] != "Changed\nContract" {
		t.Fatal(req)
	}
}
func TestSelectionBundledCallWarningAndReturn(t *testing.T) {
	m := namedOperationalFixture()
	m.openOperationalPolicies("selection")
	m.submitDialog()
	chooseBehaviorRow(t, m, "judge")
	judge := m.dialog
	chooseBehaviorRow(t, m, "selection_call_mode")
	m.dialog.index = 1
	m.submitDialog()
	if m.dialog.kind != "selection-call-bundled" || m.pending {
		t.Fatal("missing warning")
	}
	m.submitDialog()
	if m.dialog != judge {
		t.Fatal("cancel return")
	}
	chooseBehaviorRow(t, m, "selection_call_mode")
	m.dialog.index = 1
	m.submitDialog()
	m.dialog.index = 1
	req := captureCommand(t, m, m.submitDialog)
	var c map[string]any
	json.Unmarshal(req.Args["config"], &c)
	if c["selection_call_mode"] != "bundled" || m.dialog != judge {
		t.Fatal(req, m.dialog)
	}
}

func TestAssessmentResultsSeparateObservationFromPass(t *testing.T) {
	m := evalFixture()
	var record evaluationRecord
	raw := `{"title":"Trace","status":"complete","judgments":[{"status":"complete","definition":{"name":"Harm","model":"Jev","spec":"Harmful language"},"result":{"passed":false,"expected":"absent","probability":0.95,"desired_probability":0.05}},{"status":"complete","definition":{"name":"Harm","model":"LLM"},"result":{"passed":false,"expected":"absent","observed":true}}]}`
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatal(err)
	}
	text := m.collectionItemView(&record, 100)
	for _, want := range []string{"Pass when: Absent", "Presence probability: 95.0%", "Desired outcome probability: 5.0%", "Observed: Present"} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
}
func TestExplicitEmptySelectionBehaviorsStayEmpty(t *testing.T) {
	m := namedOperationalFixture()
	m.data.OperationalPolicies["selection"][0].Config["selection_behaviors"] = []map[string]any{}
	m.openOperationalPolicies("selection")
	m.submitDialog()
	if len(m.selectionBehaviors()) != 0 {
		t.Fatal("invented criteria")
	}
}
func TestJudgeCallModeQuickToggleStillWarns(t *testing.T) {
	for _, selection := range []bool{false, true} {
		m := evalFixture()
		if selection {
			m = namedOperationalFixture()
			m.openOperationalPolicies("selection")
			m.submitDialog()
			chooseBehaviorRow(t, m, "judge")
		} else {
			m.openPolicyJudge("policy", 0)
		}
		for n, r := range m.dialog.rows {
			if r.id == "call_mode" || r.id == "selection_call_mode" {
				m.dialog.index = n
			}
		}
		m.dialogKey(tea.KeyPressMsg{Code: tea.KeySpace})
		if m.pending || !strings.Contains(m.dialog.kind, "bundled") {
			t.Fatal("quick toggle bypassed warning", m.dialog)
		}
	}
}
