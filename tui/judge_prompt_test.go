package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func selectionPromptFixture(t *testing.T) *model {
	t.Helper()
	m := namedOperationalFixture()
	m.data.OperationalPolicies["selection"][0].Config["policy_prompt"] = "Original response contract"
	m.openOperationalPolicies("selection")
	m.submitDialog()
	chooseBehaviorRow(t, m, "judge")
	chooseBehaviorRow(t, m, "prompt")
	if m.editing != "policy_prompt" {
		t.Fatal("prompt editor not open")
	}
	return m
}
func TestSelectionTemplateConfirmationPreservesDraftAndOffTarget(t *testing.T) {
	m := selectionPromptFixture(t)
	text := "Changed contract\nSecond line"
	m.editor.SetValue(text)
	m.editor.MoveToEnd()
	line, col := m.editor.Line(), m.editor.Column()
	parent := m.editReturn
	if cmd := m.saveEditor(); cmd != nil || m.pending || m.editRequest != "" {
		t.Fatal("request before confirmation")
	}
	if m.dialog.kind != "judge-prompt-confirm" || m.dialog.index != 0 {
		t.Fatal("missing default cancel")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.dialog != nil || m.editReturn != parent || m.editor.Value() != text || m.editor.Line() != line || m.editor.Column() != col {
		t.Fatal("cancel lost draft")
	}
	m.saveEditor()
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.dialog != nil || m.editing != "policy_prompt" || m.editor.Value() != text {
		t.Fatal("Escape lost editor")
	}
	m.saveEditor()
	m.dialog.index = 1
	req := captureCommand(t, m, func() tea.Cmd { return m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter}) })
	if req.Command != "operational.policy.save" || string(req.Args["id"]) != `"selection-draft"` {
		t.Fatal(req)
	}
	var c map[string]any
	json.Unmarshal(req.Args["config"], &c)
	if c["policy_prompt"] != text {
		t.Fatal(c)
	}
	if m.data.OperationalPolicies["selection"][0].Config["selection_enabled"] != false {
		t.Fatal("editing enabled policy")
	}
	m.apply(stateEvent(t, m, "background"))
	if m.editing != "policy_prompt" || m.editReturn != parent {
		t.Fatal("unrelated state closed editor")
	}
	m.apply(event{Type: "error", ID: req.ID, Data: json.RawMessage(`{"message":"invalid contract"}`)})
	if m.editing != "policy_prompt" || m.editor.Value() != text || m.editor.Line() != line || m.editor.Column() != col {
		t.Fatal("error lost draft")
	}
	if cmd := m.saveEditor(); cmd != nil || m.dialog.kind != "judge-prompt-confirm" {
		t.Fatal("retry must confirm again")
	}
}
func TestEvaluationTemplateConfirmationAndDeletedTarget(t *testing.T) {
	m := evalFixture()
	m.data.EvaluationPolicies[0].Judges[0].Prompt = "Original"
	m.openPolicyJudge("policy", 0)
	chooseBehaviorRow(t, m, "prompt")
	m.editor.SetValue("New contract")
	if cmd := m.saveEditor(); cmd != nil || m.dialog.kind != "judge-prompt-confirm" {
		t.Fatal("no confirmation")
	}
	m.dialog.index = 1
	req := captureCommand(t, m, m.submitDialog)
	if req.Command != "evaluation.policy.save" {
		t.Fatal(req)
	}
	var judges []evaluationJudge
	json.Unmarshal(req.Args["judges"], &judges)
	if judges[0].Prompt != "New contract" {
		t.Fatal(judges)
	}
	m.apply(event{Type: "error", ID: req.ID, Data: json.RawMessage(`{"message":"no"}`)})
	m.saveEditor()
	m.data.EvaluationPolicies = nil
	m.dialog.index = 1
	m.submitDialog()
	if m.pending || m.editRequest != "" || m.dialog != nil || m.editor.Value() != "New contract" {
		t.Fatal("deleted target damaged draft")
	}
}
func TestUnchangedTemplateDoesNotWarn(t *testing.T) {
	m := selectionPromptFixture(t)
	previous, _ := m.currentJudgePrompt()
	m.editor.SetValue(previous)
	req := captureCommand(t, m, m.saveEditor)
	if req.Command != "operational.policy.save" || m.dialog != nil {
		t.Fatal(req)
	}
	m.apply(stateEvent(t, m, req.ID))
	if m.editing != "" || m.dialog == nil || m.dialog.title != "Selection judge" {
		t.Fatal("save did not return to judge")
	}
}

func TestTemplateConfirmationCannotFilterAwayActions(t *testing.T) {
	m := selectionPromptFixture(t)
	m.editor.SetValue("Changed contract")
	m.saveEditor()
	m.dialogKey(tea.KeyPressMsg{Code: 'z', Text: "zzzz-no-match"})
	if len(m.dialog.rows) != 2 || m.dialog.query != "" {
		t.Fatal("confirmation filtered")
	}
	m.dialogKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.pending || m.editRequest != "" || m.dialog != nil || m.editor.Value() != "Changed contract" {
		t.Fatal("typing then Enter dispatched or lost draft")
	}
	m.saveEditor()
	m.dialog.rows = nil
	if cmd := m.submitJudgePromptConfirmation(); cmd != nil || m.pending {
		t.Fatal("empty confirmation dispatched")
	}
}
func TestTemplateConfirmationRejectsJudgeChangedToClassifier(t *testing.T) {
	m := evalFixture()
	m.openPolicyJudge("policy", 0)
	chooseBehaviorRow(t, m, "prompt")
	m.editor.SetValue("New contract")
	m.saveEditor()
	m.data.EvaluationPolicies[0].Judges[0].Kind = "diffusion"
	m.apply(stateEvent(t, m, "background"))
	m.dialog.index = 1
	m.submitDialog()
	if m.pending || m.editRequest != "" || m.dialog != nil || m.editor.Value() != "New contract" {
		t.Fatal("saved LLM prompt on classifier")
	}
}

func TestJudgeTemplateEditorHeadings(t *testing.T) {
	selection := selectionPromptFixture(t)
	evaluation := evalFixture()
	evaluation.openPolicyJudge("policy", 0)
	chooseBehaviorRow(t, evaluation, "prompt")
	for _, m := range []*model{selection, evaluation} {
		m.width, m.height = 120, 36
		m.reflow()
		heading := "Prompt template"
		if m.editing == "policy_prompt" {
			heading = "Choice template"
		}
		if !strings.Contains(ansi.Strip(m.View().Content), m.editReturn.title+" · "+heading) {
			t.Fatal("missing judge template heading", ansi.Strip(m.View().Content))
		}
	}
}
