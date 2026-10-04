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
	for _, want := range []string{"historical run", "Monitoring · error", "chosen", "Fits criteria", "judge unavailable"} {
		if !strings.Contains(inspectionReportText(root), want) {
			t.Fatal(want, root.text)
		}
	}
	if strings.Contains(inspectionReportText(root), "TOKEN_SENTINEL") || strings.Contains(root.children[0].text, "TOKEN_SENTINEL") {
		t.Fatal("stream leaked")
	}
	found := false
	for _, page := range root.children {
		if strings.Contains(page.text, "TOKEN_SENTINEL") {
			found = true
		}
	}
	if !found {
		t.Fatal("raw stream lost")
	}

}

func TestInspectionReturnsThroughSectionsAndRestoresFocus(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {60, 18}} {
		m := fixture()
		m.width, m.height = size[0], size[1]
		m.focus = 0
		m.openInspection([]byte(`{"kind":"document","node":{"id":"doc","status":"complete"}}`))
		if m.dialog != nil || !m.showInspector || m.focus != 2 {
			t.Fatal("report did not open directly")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.dialog == nil {
			t.Fatal("raw evidence unreachable")
		}
		m.submitDialog()
		if m.dialog != nil || m.inspectionParent == nil {
			t.Fatal("raw record not open")
		}
		m.backFromInspection()
		m.closeDialog()
		if m.dialog != nil || !m.showInspector || !strings.Contains(m.inspection, "Not recorded") {
			t.Fatal("raw escape lost report")
		}
		m.backFromInspection()
		if m.focus != 0 || m.showInspector {
			t.Fatal("source focus not restored")
		}
	}
}

func TestInspectionScopedConversationAndVerdict(t *testing.T) {
	var data map[string]any
	json.Unmarshal([]byte(`{"kind":"simulation","id":"run","inspected_conversation":3,"conversations":[{"index":3,"name":"convo-4","status":"complete","turns":[{"role":"character","text":"only this conversation","monitor":{"status":"complete","scores":{"looping":0.1}}}]}],"evaluations":[{"status":"failed","passed":false,"error":"judge transport error"}]}`), &data)
	tree := inspectionTree(data)
	if !strings.Contains(tree.title, "convo-4") {
		t.Fatal("scope lost")
	}
	text := inspectionReportText(tree)
	if !strings.Contains(text, "Status: failed") || !strings.Contains(text, "Error: judge transport error") {
		t.Fatal(text)
	}
}

func TestInspectionDisplaysRecordedPolicyFlags(t *testing.T) {
	var data map[string]any
	json.Unmarshal([]byte(`{"kind":"document","node":{"id":"doc","policy_config":{"monitor_mode":"off","selection_enabled":true},"model":{"name":"Base model"}}}`), &data)
	text := inspectionTree(data).text
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
	text := inspectionReportText(tree)
	for _, want := range []string{"Coherence", "Model: judge", "Verdict: FAIL", "Reason: Repeated reply", "Evidence: Again"} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
	if strings.ContainsAny(inspectionHeading("évaluation\n\x1b[31mtest"), "\n\x1b") {
		t.Fatal("unsafe heading")
	}
}
func TestInspectionRetryRestoresSourceFocus(t *testing.T) {
	m := fixture()
	m.focus = 0
	m.openInspection([]byte(`{"kind":"selection","id":"attempt","steps":[{"loop":1}]}`))
	m.openInspectionRaw()
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
	if root.title != "Inspect · convo-1-loom-2" || !strings.Contains(root.text, "Conversation: 1 · convo-1-loom-2") {
		t.Fatal(root.title, root.text)
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
	m.cycleInspectionTab(1) // Evaluations, without a section menu.
	if m.dialog != nil || !m.showInspector || !strings.Contains(m.inspection, "Coherent") {
		t.Fatal("saved run report missing", m.status)
	}
}

func TestFlatInspectionShowsPromptSettingsAndFindings(t *testing.T) {
	var data map[string]any
	json.Unmarshal([]byte(`{"kind":"document","node":{"label":"doc-1","status":"complete","monitor_checks":[{"status":"complete","scores":{"looping":0.9}}]},"generation":{"prompt":"First line\nSecond line","settings":{"tokens":128}}}`), &data)
	report := inspectionTree(data)
	for _, want := range []string{"First line\nSecond line", "Tokens: 128", "Looping: 0.9", "Not recorded: selection, evaluations."} {
		if !strings.Contains(inspectionReportText(report), want) {
			t.Fatal(want, report.text)
		}
	}
	if strings.Contains(inspectionReportText(report), "── Selection") || strings.Contains(inspectionReportText(report), "── Evaluations") {
		t.Fatal("empty sections shown")
	}
}

func TestInspectionTabsKeepIndependentScroll(t *testing.T) {
	for _, size := range [][2]int{{120, 36}, {60, 18}} {
		m := fixture()
		m.width, m.height = size[0], size[1]
		m.focus = 0
		raw, _ := json.Marshal(map[string]any{"kind": "document", "node": map[string]any{"label": "doc", "status": "complete"}, "generation": map[string]any{"prompt": strings.Repeat("prompt line\n", 80)}, "evaluations": []any{map[string]any{"status": "complete", "result": map[string]any{"passed": true, "reason": "Coherent"}}}})
		m.openInspection(raw)
		m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if m.inspectionRoot.tabs[m.inspectionTab].title != "Generation" || m.dialog != nil {
			t.Fatal("Right did not switch tab")
		}
		m.inspector.SetYOffset(20)
		offset := m.inspector.YOffset()
		m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if !strings.Contains(m.inspection, "Coherent") {
			t.Fatal("evaluation inaccessible")
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
		if m.inspector.YOffset() != offset {
			t.Fatal("tab lost scroll position")
		}
		if !strings.Contains(m.inspectionTabBar(45), "[Generation]") {
			t.Fatal("active tab hidden")
		}
		m.openInspectionRaw()
		m.submitDialog()
		m.backFromInspection()
		m.closeDialog()
		if m.inspector.YOffset() != offset || m.inspectionRoot.tabs[m.inspectionTab].title != "Generation" {
			t.Fatal("raw return lost tab or scroll")
		}
	}
}

func inspectionReportText(root *inspectionPage) string {
	text := ""
	for _, tab := range root.tabs {
		text += "\n── " + tab.title + " ──\n" + tab.text
	}
	return text
}

func TestInspectorUsesDocumentPaneAtWideAndCompactWidths(t *testing.T) {
	for _, width := range []int{60, 144} {
		m := fixture()
		m.section = 1
		m.width, m.height = width, 36
		m.openInspection([]byte(`{"kind":"document","node":{"label":"doc"}}`))
		panels := m.layout().panels
		if m.focus != 2 || len(panels) > 2 {
			t.Fatal("inspector took an extra pane or lost focus")
		}
		if width == 60 && (len(panels) != 1 || panels[0].kind != 2) {
			t.Fatal("compact inspector squeezed behind tree")
		}
	}
}
