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
	m.data.EvaluationSets = []evaluationCollection{{ID: "set", Name: "Coherence", Judges: []string{"rubric"}}}
	m.data.ActiveEvaluation = "set"
	m.data.EvaluationPolicies = []evaluationPolicy{{ID: "policy", Name: "Coherence", Judges: []evaluationJudge{{ID: "judge", Name: "LLM", Kind: "llm", Model: "judge"}}, Behaviors: []evaluationBehavior{{ID: "rubric", Name: "Coherence", Spec: "Coherent writing", Threshold: .8, Enabled: true}}, Revision: 1}}
	m.data.ActiveEvaluationPolicy = "policy"
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
		if m.dialog.kind != "eval-policy-list" || m.dialog.parent != parent {
			t.Fatal("missing evaluation definitions")
		}
		m.submitDialog()
		if m.dialog.kind != "eval-policy-config" {
			t.Fatal(m.dialog)
		}
		m.closeDialog()
		if m.dialog.kind != "eval-policy-list" {
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
	m.data.SimulationRuns = []runSummary{{ID: "run", Count: 2, Status: "complete", Conversations: []simulationConversation{{Index: 0}, {Index: 1}}}}
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
	m.selectLoomConversation(map[string]any{"run": "run", "conversation": 1})
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
		if req.Command != "evaluation.collection.run" || (strings.Contains(input, "true") && string(req.Args["train_on_pass"]) != "true") || m.evalArea != "runs" || string(req.Args["policy"]) != `"policy"` {
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
	m.selected = 4
	m.toggleTarget()
	if !m.evalSelection["two"] {
		t.Fatal("space did not select result")
	}
	m.evalFilter = "training"
	m.selected = 4
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
	m.selected = 4
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
	go func() { done <- runPrimaryCommand(cmd) }()
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
	m.selected = 4
	m.focus = 1
	m.data.EvaluationSets[0].Items = []evaluationSummary{{ID: "one", Status: "complete"}}
	m.openEval("/eval")
	m.dialog.index = 4
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
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

func TestDiffusionJudgeEndpointSurvivesEditing(t *testing.T) {
	m := evalFixture()
	j := evaluationJudge{ID: "local", Name: "Voice", Kind: "diffusion", Model: "openjev-latest", Endpoint: "http://127.0.0.1:8080"}
	m.data.EvaluationPolicies[0].Judges = []evaluationJudge{j}
	m.openPolicyJudge("policy", 0)
	for _, r := range m.dialog.rows {
		if r.id == "endpoint" {
			t.Fatal("managed server leaked into configuration")
		}
	}
	m.submitDialog()
	m.dialog.fields[0].input.SetValue("Renamed")
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var judges []evaluationJudge
	json.Unmarshal(req.Args["judges"], &judges)
	if judges[0].Endpoint != j.Endpoint {
		t.Fatal("endpoint lost on rename")
	}
}

func TestEvaluateDataPoliciesRunsNavigation(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.focus = 0
	rows := m.rows()
	if len(rows) != 4 || rows[0].label != "Data" || rows[1].label != "Policies" || rows[2].label != "Runs" || rows[3].label != "+ New run" {
		t.Fatalf("root: %#v", rows)
	}
	m.evaluationCollectionAction("eval-area", "data")
	if m.rows()[1].label != "+ New dataset" {
		t.Fatal(m.rows())
	}
	m.evaluationCollectionAction("eval-collection", "set")
	m.evaluationCollectionAction("eval-back", "")
	if m.evalArea != "data" || m.evalCollection != "" {
		t.Fatal("back skipped Data")
	}
	m.evaluationCollectionAction("eval-back", "")
	if m.evalArea != "" {
		t.Fatal("back missed root")
	}
	m.openPolicy()
	if m.dialog.kind != "policy" {
		t.Fatal("Evaluate /policy must use the global hub")
	}
	m.dialog.index = 2
	m.submitDialog()
	if m.dialog.kind != "eval-policy-list" {
		t.Fatal("Evals must use same policy editor")
	}
}

func TestPolicyBehaviorsAndJudgeAreSiblings(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.openEvaluationPolicy("policy")
	root := m.dialog
	m.dialog.index = 1
	m.submitDialog()
	if m.dialog.kind != "eval-policy-behaviors" {
		t.Fatal(m.dialog)
	}
	m.submitDialog()
	if m.dialog.kind != "eval-policy-behavior" {
		t.Fatal(m.dialog)
	}
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	var behaviors []evaluationBehavior
	json.Unmarshal(req.Args["behaviors"], &behaviors)
	if req.Command != "evaluation.policy.save" || len(behaviors) != 1 || behaviors[0].Enabled {
		t.Fatal(req)
	}
	m.dialog = root
	m.dialog.index = 2
	m.submitDialog()
	if m.dialog.kind != "eval-policy-judge" {
		t.Fatal("open the policy judge directly")
	}
	for _, r := range m.dialog.rows {
		if r.id == "behaviors" {
			t.Fatal("judge must not own behaviors")
		}
	}
	if _, ok := req.Args["collection"]; ok {
		t.Fatal("policy changed data")
	}
}

func TestEvaluationRunUsesFrozenResults(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalArea = "runs"
	run := evaluationRun{ID: "result", Status: "complete", Count: 1, Completed: 1}
	run.Policy.Name = "Original policy"
	run.Policy.Revision = 1
	run.Results = []evaluationRecord{{ID: "item", Title: "Original document", Text: "Frozen source", Status: "complete"}}
	run.Results[0].Definition = evaluator{Name: "Coherence", Spec: "Clear argument"}
	run.Results[0].Result.Reason = "A coherent argument"
	run.Results[0].Result.Evidence = "Quoted evidence"
	m.data.EvaluationRuns = []evaluationRun{run}
	m.selected = 1
	req := captureCommand(t, m, func() tea.Cmd { return m.activate() })
	if req.Command != "evaluation.run.open" {
		t.Fatal(req)
	}
	payload, _ := json.Marshal(run)
	m.apply(event{Type: "evaluation_run", Data: payload})
	m.data.EvaluationPolicies[0].Name = "Changed policy"
	view := m.evaluationView(80)
	if !strings.Contains(view, "Original policy") || !strings.Contains(view, "Frozen source") || !strings.Contains(view, "A coherent argument") || !strings.Contains(view, "Quoted evidence") || strings.Contains(view, "Changed policy") {
		t.Fatal(view)
	}
}

func TestExplicitPolicyDoesNotSkipDataCompletedUnderAnotherPolicy(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalCollection = "set"
	m.selected = 0
	m.data.EvaluationSets[0].Items = []evaluationSummary{{ID: "done", Status: "complete"}}
	m.data.EvaluationPolicies = append(m.data.EvaluationPolicies, evaluationPolicy{ID: "other", Name: "Other"})
	m.openEval("/eval Other")
	m.dialog.index = 4
	req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
	if string(req.Args["items"]) != `["done"]` || string(req.Args["policy"]) != `"other"` {
		t.Fatal(req)
	}
}

func TestRunRefreshOnlyWhenSummaryChanges(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalArea = "runs"
	m.selected = 1
	r := evaluationRun{ID: "r", Status: "running", Count: 2}
	m.evalRun = &r
	m.data.EvaluationRuns = []evaluationRun{r}
	if m.evaluationRunStale("r") {
		t.Fatal("unchanged run triggered refresh")
	}
	m.data.EvaluationRuns[0].Completed = 1
	req := captureCommand(t, m, func() tea.Cmd { return m.previewTarget() })
	if req.Command != "evaluation.run.open" {
		t.Fatal(req)
	}
}
func TestExportCannotSilentlyUseDataFromRuns(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalArea = "runs"
	if cmd := m.exportItems(); cmd != nil || !strings.Contains(m.status, "data collection") {
		t.Fatal(m.status)
	}
}

func TestRunItemAggregatesFrozenBehaviorVerdicts(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalArea = "runs"
	m.selected = 1
	run := evaluationRun{ID: "r", Status: "complete"}
	run.Policy.Name = "Quality"
	one := evaluationRecord{ID: "one", Title: "doc-1", Text: "saved", Status: "complete"}
	one.Result.Passed = true
	two := one
	two.ID = "two"
	run.Results = []evaluationRecord{one, two}
	m.evalRun = &run
	m.data.EvaluationRuns = []evaluationRun{run}
	if text := m.evaluationRunView(80); !strings.Contains(text, "PASS · doc-1") || strings.Contains(text, "EVIDENCE · doc-1") {
		t.Fatal(text)
	}
	run.Results[1].Result.Passed = false
	if text := m.evaluationRunView(80); !strings.Contains(text, "FAIL · doc-1") {
		t.Fatal(text)
	}
	run.Results[1].Status = "failed"
	if text := m.evaluationRunView(80); !strings.Contains(text, "INCOMPLETE · doc-1") {
		t.Fatal(text)
	}
}
func TestRunRowsHaveDistinctStableOrdinals(t *testing.T) {
	m := evalFixture()
	m.evalArea = "runs"
	pass := true
	r := evaluationRun{ID: "one", Status: "complete", Passed: &pass}
	r.Policy.Name = "Quality"
	m.data.EvaluationRuns = []evaluationRun{r, r}
	rows := m.evaluationAreaRows()
	if rows[1].label != "2 · Quality · PASS" || rows[2].label != "1 · Quality · PASS" {
		t.Fatal(rows)
	}
}

func TestCollectionRemovalConfirmationAndReply(t *testing.T) {
	for _, fromSettings := range []bool{false, true} {
		m := evalFixture()
		m.section, m.focus = 4, 0
		m.evalArea = "data"
		if fromSettings {
			m.enterCollection("set")
			m.openCollectionConfig()
			m.dialog.index = 1
			m.submitDialog()
		} else {
			m.selected = 2
			m.focus = 3
			m.command.SetValue("/delete")
			suggestions := m.commandChoices()
			found := false
			for _, a := range suggestions {
				if a.id == "remove" {
					found = true
				}
			}
			if !found {
				t.Fatal("collection removal missing from palette", suggestions)
			}
			m.perform("remove")
		}
		d := m.dialog
		if d == nil || d.kind != "eval-collection-delete" || d.index != 0 {
			t.Fatal("missing safe confirmation", d)
		}
		m.submitDialog()
		if m.dialog != d.parent || len(m.data.EvaluationSets) != 1 {
			t.Fatal("cancel removed data or lost parent")
		}
		m.confirmRemoveCollection("set")
		m.dialog.index = 1
		req := captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
		if req.Command != "evaluation.collection.delete" || string(req.Args["collection"]) != `"set"` {
			t.Fatal(req)
		}
		m.apply(event{Type: "error", ID: req.ID, Data: json.RawMessage(`{"message":"Stop the active operation"}`)})
		if m.dialog == nil || m.dialog.args["error"] == nil {
			t.Fatal("error dismissed confirmation")
		}
		req = captureCommand(t, m, func() tea.Cmd { return m.submitDialog() })
		m.data.EvaluationSets = nil
		m.data.ActiveEvaluation = ""
		raw, _ := json.Marshal(m.data)
		m.apply(event{Type: "state", ID: req.ID, Data: raw})
		if m.dialog != nil || m.evalCollection != "" || m.evalArea != "data" {
			t.Fatal("did not return to Data")
		}
		if len(m.data.EvaluationPolicies) != 1 {
			t.Fatal("removal affected policies")
		}
	}
}

func TestEvaluateMouseControlsAndDataFocus(t *testing.T) {
	for _, width := range []int{60, 120} {
		click := func(m *model, index int) tea.Cmd {
			m.selected = index
			m.reflow()
			r := m.layout().panels[0].box
			_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: r.x + 4, Y: r.y + 3 + index - m.navStart(m.navigationRows(r))})
			return cmd
		}
		for _, tc := range []struct {
			index int
			kind  string
		}{{1, "eval-collection-config"}, {2, "eval-add-items"}, {3, "eval-filter"}} {
			m := evalFixture()
			m.width, m.height = width, 36
			m.section, m.focus = 4, 0
			m.evalArea = "data"
			m.enterCollection("set")
			click(m, tc.index)
			if m.dialog == nil || m.dialog.kind != tc.kind {
				t.Fatalf("width %d: click did not open %s", width, tc.kind)
			}
		}
		m := evalFixture()
		m.width, m.height = width, 36
		m.section, m.focus = 4, 0
		m.evalArea = "data"
		click(m, 2)
		if m.evalCollection != "set" {
			t.Fatal("collection click did not open")
		}
		for _, r := range m.rows() {
			if strings.Contains(r.label, "Run evaluation") || r.kind == "eval-new-run" {
				t.Fatal("execution control leaked into dataset")
			}
		}
		m.data.EvaluationSets[0].Items = []evaluationSummary{{ID: "done", Status: "complete"}}
		m.evaluation = &evaluationRecord{ID: "done"}
		click(m, 4)
		if m.focus != 0 || m.dialog != nil {
			t.Fatal("data click should focus and preview")
		}
		m.evaluationCollectionAction("eval-back", "")
		m.evaluationCollectionAction("eval-back", "")
		click(m, 3)
		if m.dialog == nil || m.dialog.kind != "eval-run-config" {
			t.Fatal("New run click did not configure")
		}

	}
}
