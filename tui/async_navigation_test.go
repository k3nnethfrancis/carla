package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCreatedPolicyOpensWithoutChangingActivePolicy(t *testing.T) {
	m := evalFixture()
	m.openEvaluationPolicies()
	m.newEvaluationPolicy(m.dialog)
	m.dialog.fields[0].input.SetValue("New policy")
	m.submitDialog()
	request := m.evalPolicyCreating
	if request == "" {
		t.Fatal("policy save was not correlated")
	}
	next := m.data
	next.EvaluationPolicies = append(next.EvaluationPolicies, evaluationPolicy{ID: "new", Name: "New policy", Revision: 1})
	raw, _ := json.Marshal(next)
	m.apply(event{Type: "state", ID: "background", Data: raw})
	if m.dialog.kind != "eval-policy-new" || m.evalPolicyCreating != request {
		t.Fatal("unrelated state completed policy creation")
	}
	m.apply(event{Type: "state", ID: request, Data: raw})
	if m.data.ActiveEvaluationPolicy != "policy" {
		t.Fatal("creating a policy changed the active policy")
	}
	if m.dialog.args["id"] != "new" {
		t.Fatalf("created new policy but opened %v (%s)", m.dialog.args["id"], m.dialog.title)
	}
}
func TestSimulatorBackgroundDoesNotStealBrowse(t *testing.T) {
	m := simulatorFixture()
	m.focus, m.selected = 0, 0
	m.simSelection = nil
	m.previewTarget()
	if m.simulation != nil {
		t.Fatal("config row did not clear preview")
	}
	run := simulationRun{ID: "background", Status: "running", Conversations: []simulationConversation{{Index: 0, Status: "running"}}}
	raw, _ := json.Marshal(run)
	m.apply(event{Type: "simulation", Data: raw})
	if m.simulation != nil || m.activeSimulation == nil || m.activeSimulation.ID != run.ID || m.simulationViews[run.ID] == nil {
		t.Fatal("background run should be cached without replacing preview")
	}
	if m.focus != 0 || m.selected != 0 || m.simSelection != nil {
		t.Fatalf("background event changed browsing: focus=%d selected=%d target=%+v", m.focus, m.selected, m.simSelection)
	}
}
func TestUnrelatedLiveEvaluationPreservesScroll(t *testing.T) {
	m := evalFixture()
	m.section, m.evalArea = 4, "runs"
	m.evalRun = &evaluationRun{ID: "reading", Status: "complete"}
	m.document.SetContent(strings.Repeat("archived report line\n", 100))
	m.document.SetYOffset(30)
	old := m.document.YOffset()
	raw, _ := json.Marshal(evaluationRun{ID: "other", Status: "running", Live: true})
	m.apply(event{Type: "evaluation_run", Data: raw})
	if m.document.YOffset() != old {
		t.Fatalf("unrelated live run reset scroll %d -> %d while selected run remains %s", old, m.document.YOffset(), m.evalRun.ID)
	}
}

func TestPolicyCreateReplyRespectsNavigationAndErrors(t *testing.T) {
	for _, fail := range []bool{false, true} {
		m := evalFixture()
		m.openEvaluationPolicies()
		parent := m.dialog
		m.newEvaluationPolicy(parent)
		m.dialog.fields[0].input.SetValue("New policy")
		m.submitDialog()
		request := m.evalPolicyCreating
		if fail {
			m.apply(event{Type: "error", ID: request, Data: json.RawMessage(`{"message":"invalid policy"}`)})
			if m.dialog.kind != "eval-policy-new" || m.dialog.fields[0].input.Value() != "New policy" {
				t.Fatal("save failure lost correctable policy form")
			}
		} else {
			m.dialog = parent // User leaves the pending form before its reply.
			next := m.data
			next.EvaluationPolicies = append(next.EvaluationPolicies, evaluationPolicy{ID: "new", Name: "New policy"})
			raw, _ := json.Marshal(next)
			m.apply(event{Type: "state", ID: request, Data: raw})
			if m.dialog.kind != parent.kind {
				t.Fatal("late create reply reopened a dismissed policy")
			}
		}
		if m.evalPolicyCreating != "" || m.evalPolicyExisting != nil {
			t.Fatal("policy creation tracking not cleared")
		}
	}
}
