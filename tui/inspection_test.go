package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectionSeparatesHistoryFromRawStream(t *testing.T) {
	raw := []byte(`{"kind":"document","node":{"id":"doc","status":"complete","text":"Source","monitor":{"status":"unavailable","error":"judge unavailable"},"trace":{"events":[{"content":"TOKEN_SENTINEL"}]}},"generation":{"id":"doc","trace":{"events":[{"content":"TOKEN_SENTINEL"}]}},"selection_runs":[{"id":"attempt","status":"complete","steps":[{"loop":1}]}],"selection_summaries":[{"id":"attempt","status":"complete","steps":[{"loop":1,"outcomes":{"doc":"chosen"},"reasons":{"doc":"Fits criteria"}}]}]}`)
	var data map[string]any
	json.Unmarshal(raw, &data)
	root := inspectionTree(data)
	if len(root.children) != 6 {
		t.Fatal(root)
	}
	overview := root.children[0].text
	if !strings.Contains(overview, "historical run") || !strings.Contains(overview, "Monitoring · error") || !strings.Contains(overview, "Selection · 1 attempt") {
		t.Fatal(overview)
	}
	for _, section := range root.children[:5] {
		if strings.Contains(section.text, "TOKEN_SENTINEL") {
			t.Fatal("transport events leaked into summary", section.title)
		}
	}
	if !strings.Contains(root.children[5].text, "TOKEN_SENTINEL") {
		t.Fatal("raw evidence lost")
	}
	selection := root.children[3]
	if !strings.Contains(selection.text, "chosen") || !strings.Contains(selection.text, "Fits criteria") {
		t.Fatal(selection.text)
	}
	if selection.children[0].retry != "attempt" {
		t.Fatal("retry missing")
	}
}

func TestInspectionReturnsThroughSectionsAndRestoresFocus(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {60, 18}} {
		m := fixture()
		m.width, m.height = size[0], size[1]
		m.focus = 0
		m.openInspection([]byte(`{"kind":"document","node":{"id":"doc","status":"complete"}}`))
		root := m.dialog
		m.submitDialog() // Overview leaf.
		if m.focus != 2 || !m.showInspector || m.dialog != nil {
			t.Fatal("overview not readable")
		}
		if !m.backFromInspection() || m.dialog != root {
			t.Fatal("escape lost root")
		}
		m.dialog.index = 1
		m.submitDialog() // Generation menu, Summary then Saved text if present.
		if m.dialog != nil {
			m.closeDialog()
		}
		if m.dialog == nil {
			m.backFromInspection()
		}
		for m.dialog != nil {
			m.closeDialog()
		}
		if m.focus != 0 || m.showInspector {
			t.Fatal("invisible inspector focus", m.focus)
		}
	}
}

func TestInspectionScopedConversationAndVerdict(t *testing.T) {
	var data map[string]any
	json.Unmarshal([]byte(`{"kind":"simulation","id":"run","inspected_conversation":3,"conversations":[{"index":3,"name":"convo-4","status":"complete","turns":[{"role":"character","text":"only this conversation","monitor":{"status":"complete","scores":{"looping":0.1}}}]}],"evaluations":[{"status":"failed","passed":false,"error":"judge transport error"}]}`), &data)
	tree := inspectionTree(data)
	if len(tree.children[1].children) != 1 || tree.children[1].children[0].title != "convo-4" {
		t.Fatal("conversation scope lost")
	}
	text := tree.children[4].children[0].text
	if !strings.Contains(text, "Status: failed") || !strings.Contains(text, "Error: judge transport error") {
		t.Fatal(text)
	}
}

func TestInspectionDisplaysRecordedPolicyFlags(t *testing.T) {
	var data map[string]any
	json.Unmarshal([]byte(`{"kind":"document","node":{"id":"doc","policy_config":{"monitor_mode":"off","selection_enabled":true},"model":{"name":"Base model"}}}`), &data)
	text := inspectionTree(data).children[0].text
	if !strings.Contains(text, "Saved monitoring setting: off") || !strings.Contains(text, "Saved selection enabled: true") || !strings.Contains(text, "Model: Base model") || strings.Contains(text, "historical") {
		t.Fatal(text)
	}
}

func TestInspectionMonitoringIgnoresUnmonitoredTurns(t *testing.T) {
	p := inspectionMonitoring([]any{map[string]any{"role": "character", "monitor": map[string]any{"status": "complete"}}, map[string]any{"role": "visitor"}})
	if p.title != "Monitoring · complete · 1 check" {
		t.Fatal(p.title)
	}
	p = inspectionMonitoring([]any{map[string]any{"monitor": map[string]any{"status": "error"}}, map[string]any{"monitor": map[string]any{"status": "checking"}}})
	if !strings.Contains(p.title, "error") {
		t.Fatal(p.title)
	}
	if inspectionLabel("évaluation_name") != "Évaluation name" {
		t.Fatal("unicode label corrupted")
	}
}

func TestInspectionEvaluationSummaryUsesJudgmentAndBatchResults(t *testing.T) {
	var data map[string]any
	json.Unmarshal([]byte(`{"id":"eval-run","status":"complete","passed":false,"results":[{"status":"complete","definition":{"name":"Coherence","model":"judge"},"result":{"passed":false,"reason":"Repeated reply","evidence":"Again"}},{"status":"failed","error":"Unavailable"}]}`), &data)
	tree := inspectionTree(data)
	if tree.children[4].title != "Evaluations · 2 results" {
		t.Fatal(tree.children[4].title)
	}
	text := tree.children[4].children[0].text
	for _, want := range []string{"Coherence", "Model: judge", "Verdict passed: false", "Reason: Repeated reply", "Evidence: Again"} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
	m := fixture()
	m.showInspectionPage(&inspectionPage{title: "évaluation\n\x1b[31mtest", children: []*inspectionPage{{title: "raw\nkey"}}}, nil)
	if strings.ContainsAny(m.dialog.title, "\n\x1b") || strings.Contains(m.dialog.rows[0].label, "\n") {
		t.Fatal("unsafe dialog labels")
	}
}
func TestInspectionRetryRestoresSourceFocus(t *testing.T) {
	m := fixture()
	m.focus = 0
	m.openInspection([]byte(`{"kind":"selection","id":"attempt","steps":[{"loop":1}]}`))
	m.dialog.index = 3
	m.submitDialog()
	m.dialog.index = 1
	m.submitDialog()
	m.dialog.index = 0
	m.submitDialog()
	m.backFromInspection()
	m.dialog.index = len(m.dialog.rows) - 1
	m.submitDialog()
	m.dialog.index = 1
	m.submitDialog()
	if m.focus != 0 || m.showInspector || m.inspectionParent != nil || m.dialog != nil {
		t.Fatal("retry left hidden inspector focus")
	}
}

func TestInspectionUsesSavedLabelsAndOneBasedConversation(t *testing.T) {
	var data map[string]any
	json.Unmarshal([]byte(`{"kind":"document","node":{"id":"uuid","title":"","label":"continue-1-doc-1"}}`), &data)
	if title := inspectionTree(data).title; title != "Inspect · continue-1-doc-1" {
		t.Fatal(title)
	}
	data = nil
	json.Unmarshal([]byte(`{"kind":"simulation","id":"run","title":"Parent loom","inspected_conversation":0,"conversations":[{"index":0,"label":"convo-1-loom-2","turns":[]}]}`), &data)
	root := inspectionTree(data)
	if root.title != "Inspect · convo-1-loom-2" || !strings.Contains(root.children[0].text, "Conversation: 1 · convo-1-loom-2") {
		t.Fatal(root.title, root.children[0].text)
	}
}

func TestInspectEvaluationRunThroughCommandBar(t *testing.T) {
	m := evalFixture()
	m.section = 4
	m.evalArea = "runs"
	m.focus = 3
	m.selected = 1
	run := evaluationRun{ID: "run", Status: "complete", Count: 1, Completed: 1}
	m.data.EvaluationRuns = []evaluationRun{run}
	m.evalRun = &run
	m.evalRunRaw = []byte(`{"id":"run","status":"complete","results":[{"status":"complete","result":{"passed":true,"reason":"Coherent"}}]}`)
	m.command.SetValue("/inspect")
	choices := m.commandChoices()
	if len(choices) != 1 || choices[0].id != "inspect" {
		t.Fatal("saved run inspect missing from commands", choices)
	}
	m.commandKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog == nil || m.dialog.kind != "inspection" {
		t.Fatal("slash command did not open inspector", m.status)
	}
	root := m.dialog.args["page"].(*inspectionPage)
	if len(root.children[4].children) != 1 || !strings.Contains(root.children[4].children[0].text, "Coherent") {
		t.Fatal("saved run evidence missing")
	}
}
