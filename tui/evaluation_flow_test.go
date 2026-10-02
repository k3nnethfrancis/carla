package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestDatasetCreationOpensAddAndAnthologyComesFirst(t *testing.T) {
	m := evalFixture()
	m.section, m.evalArea = 4, "data"
	m.data.Nodes = []node{{ID: "other", Title: "Other"}, {ID: "kept", Title: "Kept", Kept: true}}
	m.evaluationCollectionAction("eval-create", "")
	m.dialog.fields[0].input.SetValue("New samples")
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	m.data.EvaluationSets = append(m.data.EvaluationSets, evaluationCollection{ID: "new", Name: "New samples"})
	m.data.ActiveEvaluation = "new"
	raw, _ := json.Marshal(m.data)
	m.apply(event{Type: "state", ID: req.ID, Data: raw})
	if m.evalCollection != "new" || m.dialog == nil || m.dialog.kind != "eval-add-items" {
		t.Fatal("creation must open Add")
	}
	if m.dialog.rows[0].id != "node:kept" || !strings.HasPrefix(m.dialog.rows[0].label, "Anthology") || m.dialog.rows[1].id != "node:other" {
		t.Fatal(m.dialog.rows)
	}
	m.dialog = nil
	m.openCollectionConfig()
	for _, r := range m.dialog.rows {
		if r.id == "active" {
			t.Fatal("dataset default must not be offered")
		}
	}
	m.dialog = nil
	m.enterCollection("")
	m.selected = 0
	m.newEvaluationRun("", nil)
	if m.dialog.args["draft"].(*evaluationRunDraft).Dataset != "" {
		t.Fatal("must choose dataset explicitly")
	}
}

func TestLiveEvaluationOpensRunAndRetainsProgress(t *testing.T) {
	m := evalFixture()
	m.section = 1
	m.apply(event{Type: "evaluation_run", Data: json.RawMessage(`{"id":"live","opened":true,"status":"running","policy":{"name":"Quality"},"count":1}`)})
	if m.section != 4 || m.evalArea != "runs" || m.focus != 1 || m.targetRow().id != "live" {
		t.Fatal("run must be selected automatically")
	}
	for _, text := range []string{"first ", "second"} {
		raw, _ := json.Marshal(evaluationProgress{Run: "live", Record: "r", Judge: "Judge", Stage: "Generating judgment", Text: text, Call: 1, Total: 1})
		m.apply(event{Type: "evaluation_progress", Data: raw})
	}
	if !strings.Contains(m.evaluationRunView(80), "first second") {
		t.Fatal("missing live output")
	}
	m.apply(event{Type: "evaluation_run", Data: json.RawMessage(`{"id":"live","status":"incomplete","results":[{"status":"failed","error":"Judge unavailable","text":"source"}]}`)})
	if !strings.Contains(m.evaluationRunView(80), "Judge unavailable") {
		t.Fatal("failure must be readable in Runs")
	}
}

func TestLiveRunDoesNotStealExplicitRunNavigation(t *testing.T) {
	m := evalFixture()
	m.evalRun = &evaluationRun{ID: "older"}
	m.apply(event{Type: "evaluation_run", Data: json.RawMessage(`{"id":"background","live":true,"status":"running"}`)})
	if m.evalRun.ID != "older" {
		t.Fatal("background update stole opened run")
	}
	m.apply(event{Type: "evaluation_run", Data: json.RawMessage(`{"id":"chosen","status":"complete"}`)})
	if m.evalRun.ID != "chosen" {
		t.Fatal("explicit run open was blocked")
	}
}
