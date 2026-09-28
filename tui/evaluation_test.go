package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
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
	m.data.EvaluationSets = []evaluationCollection{{ID: "set", Name: "Coherence", Judges: []string{"rubric"}}}
	m.data.ActiveEvaluation = "set"
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
	if len(targets) != 0 {
		t.Fatal(targets)
	}
	m.loomConversation = &conversationParent{Run: "run", Conversation: 1}
	if targets = m.evaluationTargets(); len(targets) != 1 || targets[0]["conversation"] != 1 {
		t.Fatal(targets)
	}
}
func TestEvalCommandDispatchAndDefaults(t *testing.T) {
	for _, input := range []string{"/eval", "/eval Coherence --train-on-pass true"} {
		m := evalFixture()
		m.section = 1
		m.branchSelection = map[string]bool{m.currentID(): true}
		req := captureCommand(t, m, func() tea.Cmd { return m.openEval(input) })
		if req.Command != "evaluation.collection.run" || string(req.Args["train_on_pass"]) != fmt.Sprint(strings.Contains(input, "true")) || m.evalCollection != "set" {
			t.Fatal(req)
		}
	}
	for _, input := range []string{"/eval --train-on-pass", "/eval --train-on-pass yes"} {
		if _, _, err := parseEvalOptions(input); err == nil {
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
	m.evalCollection = "set"
	m.data.EvaluationSets[0].Items = []evaluationSummary{{ID: "one", Title: "Document", Status: "complete", Passed: &passed, Training: true}, {ID: "two", Title: "Conversation", Status: "complete", Passed: &failed}}
	m.selected = 5
	m.toggleTarget()
	if !m.evalSelection["two"] {
		t.Fatal("space did not select result")
	}
	m.evalFilter = "training"
	m.selected = 5
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
		if !strings.Contains(frame, "Evaluate") {
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
	m.selected = 5
	m.evalCollection = "set"
	m.data.EvaluationSets[0].Items = []evaluationSummary{{ID: "one", Status: "complete"}}
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
	if request.Command != "evaluation.item.annotate" || request.Args["note"] != "New note" {
		t.Fatal(request)
	}
	if m.editing != "evaluation-note" {
		t.Fatal("draft lost before acknowledgement")
	}
}

func TestRerunFromEvaluationTabDoesNotLoseRequestToPreview(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalCollection = "set"
	m.selected = 5
	m.focus = 1
	m.data.EvaluationSets[0].Items = []evaluationSummary{{ID: "one", Status: "complete"}}
	req := captureCommand(t, m, func() tea.Cmd { return m.openEval("/eval") })
	if req.Command != "evaluation.collection.run" {
		t.Fatal(req)
	}
}

func TestNamedEvaluationLoomOptions(t *testing.T) {
	o, err := parseGenerationOptions(`/loom 3 --turns 2 --tokens 512 --eval "Character voice" --loops 4`, "loom")
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{}
	o.apply(args)
	if args["eval"] != "Character voice" || args["count"] != 3 || args["loops"] != 4 || args["turns"] != 2 || args["n_predict"] != 512 {
		t.Fatal(args)
	}
	for _, input := range []string{`/loom --eval`, `/loom --eval ""`, `/loom --eval a --eval b`} {
		if _, err := parseGenerationOptions(input, "loom"); err == nil {
			t.Fatal(input)
		}
	}
}

func TestCollectionConfigAndAddingDoNotRunJudges(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.enterCollection("set")
	m.openConfig()
	if m.dialog == nil || m.dialog.kind != "eval-collection-config" {
		t.Fatal(m.dialog)
	}
	m.closeDialog()
	m.openCollectionItems()
	if m.dialog == nil || m.dialog.kind != "eval-add-items" {
		t.Fatal(m.dialog)
	}
	if len(m.dialog.rows) == 0 {
		t.Fatal("missing source items")
	}
	m.dialog.args["selected"] = map[string]bool{m.dialog.rows[0].id: true}
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	if req.Command != "evaluation.collection.add" {
		t.Fatal(req)
	}
}
