package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Judge templates define both interpretation and the machine-readable response.
// Keep the full editor draft owned by the editor while confirmation is open.
func (m *model) saveEditor() tea.Cmd {
	if m.pending || m.data.Busy {
		return nil
	}
	if m.editing != "policy_prompt" && m.editing != "selection_assessment_prompt" && m.editing != "policy-judge-prompt" {
		return m.persistEditor()
	}
	previous, ok := m.currentJudgePrompt()
	if !ok {
		m.status = "This judge no longer exists; cancel this draft"
		return nil
	}
	text := m.editor.Value()
	if strings.TrimSpace(text) == "" {
		m.status = "The prompt template cannot be empty"
		return nil
	}
	if text == previous {
		return m.persistEditor()
	}
	warning := "Changing this template affects how the judge assesses behaviors and its required response format. Invalid output can make judging fail."
	title := "Replace judge prompt template?"
	if m.editing == "policy_prompt" {
		title = "Replace choice template?"
	}
	if m.editing == "selection_assessment_prompt" {
		title = "Replace assessment template?"
	}
	m.dialog = &dialog{kind: "judge-prompt-confirm", title: title, rows: []row{{id: "cancel", label: "Keep editing", preview: warning}, {id: "confirm", label: "Replace template", preview: warning + " Save this template for future runs."}}}
	return nil
}
func (m *model) currentJudgePrompt() (string, bool) {
	if m.editing == "policy_prompt" || m.editing == "selection_assessment_prompt" {
		purpose, id := m.operationalContext()
		if purpose == "selection" && id != "" && m.operationalPolicy(purpose, id).ID == "" {
			return "", false
		}
		return m.selectionString(m.editing), true
	}
	d := m.editReturn
	if d == nil {
		return "", false
	}
	id, _ := d.args["id"].(string)
	p := m.findEvaluationPolicy(id)
	i, ok := d.args["judge"].(int)
	if p == nil || !ok || i < 0 || i >= len(p.Judges) || p.Judges[i].Kind != "llm" {
		return "", false
	}
	if expected, ok := d.args["judge_id"].(string); ok && p.Judges[i].ID != expected {
		return "", false
	}
	return p.Judges[i].Prompt, true
}
func (m *model) resumeJudgePromptDraft() tea.Cmd {
	m.dialog = nil
	m.focus = 1
	return m.editor.Focus()
}
func (m *model) submitJudgePromptConfirmation() tea.Cmd {
	if m.dialog == nil || len(m.dialog.rows) == 0 || m.dialog.index < 0 || m.dialog.index >= len(m.dialog.rows) {
		return nil
	}
	if m.dialog.rows[m.dialog.index].id != "confirm" {
		return m.resumeJudgePromptDraft()
	}
	// Resolve the target again: a state update may arrive while confirmation is open.
	if _, ok := m.currentJudgePrompt(); !ok {
		m.status = "This judge no longer exists; cancel this draft"
		return m.resumeJudgePromptDraft()
	}
	m.dialog = nil
	m.focus = 1
	return m.persistEditor()
}
