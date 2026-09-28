package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	"net"
	"strings"
	"testing"
)

func evalFixture() *model {
	m := fixture()
	m.width, m.height = 120, 36
	m.data.Evaluators = []evaluator{{ID: "rubric", Name: "Coherence", Kind: "llm", Revision: 1, Spec: "Coherent writing", Model: "judge", Threshold: .8}}
	m.data.SelectorModels = []localModel{{Name: "Judge", Alias: "judge"}}
	m.data.EvaluationPrompt = "Judge criteria, return passed, reason and evidence."
	return m
}
func TestPolicySeparateFromGenerationSettings(t *testing.T) {
	for _, section := range []int{0, 1, 2, 3} {
		m := evalFixture()
		m.section = section
		m.openConfig()
		for _, r := range m.dialog.rows {
			if r.id == "selection" || r.id == "monitor" {
				t.Fatal("policy leaked into config")
			}
		}
		m.openPolicy()
		parent := m.dialog
		parent.index = 2
		m.submitDialog()
		if m.dialog.kind != "eval-definitions" || m.dialog.parent != parent {
			t.Fatal("missing evaluation definitions")
		}
		m.submitDialog()
		if m.dialog.kind != "eval-definition" {
			t.Fatal(m.dialog)
		}
		m.closeDialog()
		if m.dialog.kind != "eval-definitions" {
			t.Fatal("escape skipped a level")
		}
	}
}
func TestEvalTargetsUseSelectedVersionsAndExplicitConversationBatch(t *testing.T) {
	m := evalFixture()
	m.section = 1
	m.branchSelection = map[string]bool{m.data.Nodes[0].ID: true}
	if targets := m.evaluationTargets(); len(targets) != 1 || targets[0]["node"] != m.data.Nodes[0].ID {
		t.Fatal(targets)
	}
	m.section = 3
	m.simulation = nil
	m.conversationOpen = false
	m.data.SimulationRuns = []runSummary{{ID: "run", Count: 2, Status: "complete"}}
	for i, r := range m.rows() {
		if r.kind == "simulation" {
			m.selected = i
			break
		}
	}
	targets := m.evaluationTargets()
	if len(targets) != 2 || targets[1]["conversation"] != 1 {
		t.Fatal(targets)
	}
}
func TestEvalCommandDispatchAndDefaults(t *testing.T) {
	for _, input := range []string{"/eval", "/eval --train-on-pass true"} {
		m := evalFixture()
		m.section = 1
		m.focus = 3
		m.command.SetValue(input)
		left, right := net.Pipe()
		m.client = &client{conn: left}
		m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.dialog == nil || m.dialog.kind != "eval-run" {
			t.Fatal(m.status, m.commandChoices())
		}
		cmd := m.submitDialog()
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		var request struct {
			Command string
			Args    map[string]any
		}
		if err := json.NewDecoder(right).Decode(&request); err != nil {
			t.Fatal(err)
		}
		<-done
		left.Close()
		right.Close()
		if request.Command != "evaluation.run" || request.Args["train_on_pass"] != strings.Contains(input, "true") || m.section != 4 {
			t.Fatal(request, m.section)
		}
	}
	for _, input := range []string{"/eval true", "/eval --train-on-pass", "/eval --train-on-pass yes"} {
		if _, err := parseEvalOptions(input); err == nil {
			t.Fatal(input)
		}
	}
}
func TestEvaluationSelectionFilteringAndNavigation(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.focus = 0
	passed := true
	failed := false
	m.data.Evaluations = []evaluationSummary{{ID: "one", Title: "Document", Status: "complete", Passed: &passed, Training: true}, {ID: "two", Title: "Conversation", Status: "complete", Passed: &failed}}
	m.selected = 2
	m.toggleTarget()
	if !m.evalSelection["two"] {
		t.Fatal("space did not select result")
	}
	m.evalFilter = "training"
	m.selected = 2
	if m.targetRow().id != "one" {
		t.Fatal(m.rows())
	}
	m.evaluation = &evaluationRecord{ID: "one", Title: "Document", Status: "complete", Text: strings.Repeat("A path 🙂. ", 100), Training: true, Definition: m.data.Evaluators[0]}
	m.evaluation.Result.Passed = true
	for _, size := range [][2]int{{60, 18}, {80, 24}, {120, 36}} {
		m.width, m.height = size[0], size[1]
		m.focus = 1
		m.reflow()
		frame := ansi.Strip(m.View().Content)
		if !strings.Contains(frame, "Evaluation") {
			t.Fatal(frame)
		}
		if len(strings.Split(frame, "\n")) > size[1] {
			t.Fatal("height overflow")
		}
		for _, line := range strings.Split(frame, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("width overflow", line)
			}
		}
	}
	m.openPolicy()
	m.closeDialog()
	m.switchSection(1)
	m.switchSection(4) // page restoration must handle the fifth tab
}
func TestEvaluationNotesSaveDoesNotEditSource(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.selected = 2
	m.data.Evaluations = []evaluationSummary{{ID: "one", Status: "complete"}}
	m.evaluation = &evaluationRecord{ID: "one", Note: "Original"}
	m.evalAction("notes")
	m.editor.SetValue("New note")
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m.client = &client{conn: left}
	cmd := m.saveEditor()
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var request struct {
		Command string
		Args    map[string]any
	}
	json.NewDecoder(right).Decode(&request)
	<-done
	if request.Command != "evaluation.annotate" || request.Args["note"] != "New note" {
		t.Fatal(request)
	}
	if m.editing != "evaluation-note" {
		t.Fatal("draft lost before acknowledgement")
	}
}

func TestRerunFromEvaluationTabDoesNotLoseRequestToPreview(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.selected = 2
	m.focus = 1
	m.data.Evaluations = []evaluationSummary{{ID: "one", Status: "complete"}}
	m.evaluation = &evaluationRecord{ID: "one"}
	m.openEval("/eval")
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	m.client = &client{conn: left}
	cmd := m.submitDialog()
	if cmd == nil {
		t.Fatal("preview swallowed evaluation request")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var request struct {
		Command string
		Args    map[string]any
	}
	json.NewDecoder(right).Decode(&request)
	<-done
	if request.Command != "evaluation.run" {
		t.Fatal(request)
	}
	if !m.evalStarting {
		t.Fatal("new results will not be selected")
	}
}
