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
	if j.Kind != "diffusion" || j.CallMode != "separate" || len(j.Behaviors) != 0 {
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
	m.data.EvaluationPolicies[0].Judges[0].Behaviors = append(m.data.EvaluationPolicies[0].Judges[0].Behaviors, evaluationBehavior{Name: "Second", Enabled: true})
	m.closeDialog()
	if m.dialog.kind != "eval-policy-judges" || !strings.Contains(m.dialog.rows[0].preview, "2 behaviors") {
		t.Fatalf("stale parent after child edit: %+v", m.dialog)
	}
}
