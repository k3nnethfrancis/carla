package main

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// A judge owns invocation settings; behaviors describe what it assesses.
// Frozen run definitions use the same wire shape without editable references.
type evaluationJudge struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Endpoint string `json:"endpoint,omitempty"`
	Revision int    `json:"revision"`
	Missing  bool   `json:"missing,omitempty"`
	CallMode string `json:"call_mode"`
}
type evaluationBehavior struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Spec           string  `json:"spec"`
	Threshold      float64 `json:"threshold"`
	Expected       string  `json:"expected,omitempty"`
	Enabled        bool    `json:"enabled"`
	Revision       int     `json:"revision"`
	SourceID       string  `json:"source_id,omitempty"`
	SourceRevision int     `json:"source_revision,omitempty"`
}

// Legacy frozen runs contained a flat evaluator. Adapt only the display shape.
type historicalEvaluationJudge struct {
	evaluationJudge
	Behaviors []evaluationBehavior `json:"behaviors"`
}

func (j *historicalEvaluationJudge) UnmarshalJSON(data []byte) error {
	type wire historicalEvaluationJudge
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*j = historicalEvaluationJudge(w)
	if j.Behaviors == nil {
		var old evaluator
		if err := json.Unmarshal(data, &old); err != nil {
			return err
		}
		if old.Spec != "" {
			j.Behaviors = []evaluationBehavior{{ID: old.ID, Name: old.Name, Spec: old.Spec, Threshold: old.Threshold, Enabled: true, Revision: old.Revision}}
		}
	}
	return nil
}
func policyJudgeArgs(id string, j int) map[string]any { return map[string]any{"id": id, "judge": j} }
func (m *model) openEvaluationJudge(id string) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil {
		return nil
	}
	if len(p.Judges) == 1 {
		return m.openPolicyJudge(id, 0)
	}
	return m.policyModelPicker(&dialog{args: policyJudgeArgs(id, -1)})
}

func (m *model) openPolicyJudge(id string, i int) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil || i < 0 || i >= len(p.Judges) {
		return nil
	}
	j := p.Judges[i]
	mode := "Separate"
	if j.CallMode == "bundled" {
		mode = "Bundled"
	}
	rows := []row{{id: "model", label: "Model · " + m.evaluatorModelName(evaluator{Kind: j.Kind, Model: j.Model}), preview: "Choose the LLM that assesses every enabled behavior against the complete trace."}, {id: "call_mode", label: "Call mode · " + mode, preview: "Separate assesses each behavior in its own request. Bundled assesses them together."}}

	if j.Kind == "llm" {
		rows = append(rows, row{id: "prompt", label: "Prompt template", preview: "Full instructions and response contract used by this judge. Saving changes requires confirmation."})
	}
	rows = append(rows, row{id: "delete", label: "Remove judge…", preview: "Remove this judge. Policy behaviors and past results remain."})
	m.dialog = &dialog{kind: "eval-policy-judge", title: p.Name + " · Judge", args: policyJudgeArgs(id, i), rows: rows}
	m.dialog.args["judge_id"] = j.ID
	return nil
}
func (m *model) openPolicyBehaviors(id string) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil {
		return nil
	}
	d := &dialog{kind: "eval-policy-behaviors", title: p.Name + " · Behaviors", args: policyBehaviorArgs(id, -1)}
	for k, b := range p.Behaviors {
		label := b.Name
		if !b.Enabled {
			label += " · off"
		}
		d.rows = append(d.rows, row{id: strconv.Itoa(k), label: label, preview: b.Spec})
	}
	d.rows = append(d.rows, row{id: "library", label: "From library", preview: "Import Off. Review its wording for the complete trace and choose Pass when before enabling."}, row{id: "new", label: "+ New behavior", preview: "Write criteria for this policy’s judge."})
	m.dialog = d
	return nil
}
func policyBehaviorArgs(id string, k int) map[string]any {
	return map[string]any{"id": id, "behavior": k}
}
func (m *model) openPolicyBehavior(id string, k int) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil || k < 0 || k >= len(p.Behaviors) {
		return nil
	}
	b := p.Behaviors[k]
	enabled := "Off"
	if b.Enabled {
		enabled = "On"
	}
	rows := []row{
		{id: "enabled", label: "Enabled · " + enabled, preview: "Off skips this behavior for this policy while preserving its settings."},
		{id: "name", label: "Name · " + b.Name, preview: "Short name shown beside this behavior’s results."},
		{id: "spec", label: "Behavior spec", preview: "Describe what counts as this behavior in the complete document or conversation. " + b.Spec},
		{id: "expected", label: "Pass when · " + expectedLabel(b.Expected), preview: expectedHelp},
		m.behaviorLibraryRow(b),
		{id: "delete", label: "Remove behavior…", preview: "Remove from this policy. Past results retain the exact spec."},
	}
	if len(p.Judges) == 1 && p.Judges[0].Kind != "llm" {
		threshold := row{id: "threshold", label: fmt.Sprintf("Pass threshold · %.0f%%", b.Threshold*100), preview: "Minimum probability of the desired outcome. This applies to the saved classifier judge."}
		rows = append(rows[:4], append([]row{threshold}, rows[4:]...)...)
	}
	m.dialog = &dialog{kind: "eval-policy-behavior", title: b.Name, args: policyBehaviorArgs(id, k), rows: rows}
	return nil
}
func (m *model) policyModelPicker(d *dialog) tea.Cmd {
	n := &dialog{kind: "eval-policy-judge-model", title: "Model", parent: d, args: d.args}
	for _, model := range m.data.SelectorModels {
		n.rows = append(n.rows, row{id: "llm:" + model.Alias, label: model.Name + " (LLM)", preview: "Local instruction model returning structured judgments."})
	}
	m.dialog = n
	return nil
}
func (m *model) savePolicyDraft(d *dialog, p evaluationPolicy) tea.Cmd {
	return m.saveDialog(d, "evaluation.policy.save", map[string]any{"id": p.ID, "name": p.Name, "judges": p.Judges, "behaviors": p.Behaviors, "actions": p.Actions})
}
func (m *model) submitPolicyJudge(d *dialog) tea.Cmd {
	id, _ := d.args["id"].(string)
	original := m.findEvaluationPolicy(id)
	if original == nil {
		return nil
	}
	// Work on a copy until Python validates and persists the change.
	raw, _ := json.Marshal(original)
	var p evaluationPolicy
	_ = json.Unmarshal(raw, &p)
	i := -1
	if value, ok := d.args["judge"].(int); ok {
		i = value
	}
	k := -1
	if value, ok := d.args["behavior"].(int); ok {
		k = value
	}
	if i >= len(p.Judges) || (strings.HasPrefix(d.kind, "eval-policy-judge") && d.kind != "eval-policy-judge-model" && i < 0) {
		parent := policyConfigParent(d)
		m.openEvaluationJudge(id)
		m.dialog.parent = parent
		m.status = "Choose a model for this policy"
		return nil
	}
	if k >= len(p.Behaviors) || (d.kind == "eval-policy-behavior" || d.kind == "eval-policy-behavior-field" || d.kind == "eval-policy-behavior-delete") && k < 0 {
		parent := policyConfigParent(d)
		m.openPolicyBehaviors(id)
		m.dialog.parent = parent
		m.status = "Behavior no longer exists; choose a behavior"
		return nil
	}
	if d.kind == "eval-policy-behavior-field" {
		value := strings.TrimSpace(d.fields[0].input.Value())
		if value == "" {
			m.status = "Enter a value"
			return nil
		}
		field := d.args["field"].(string)
		if field == "threshold" {
			v, err := strconv.ParseFloat(value, 64)
			if err != nil || v <= 0 || v > 100 {
				m.status = "Enter a percentage greater than 0 and at most 100"
				return nil
			}
			p.Behaviors[k].Threshold = v / 100
		} else {
			p.Behaviors[k].Name = value
		}
		return m.savePolicyDraft(d, p)
	}
	if d.kind == "eval-policy-behavior-new" {
		name := strings.TrimSpace(d.fields[0].input.Value())
		if name == "" {
			m.status = "Name the behavior"
			return nil
		}
		m.editing = "policy-behavior-new"
		m.editReturn = d
		m.dialog = nil
		m.editor.SetValue("")
		m.focus = 1
		m.reflow()
		return m.editor.Focus()
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	child := func(cmd tea.Cmd) tea.Cmd {
		if m.dialog != nil {
			m.dialog.parent = d
		}
		return cmd
	}
	switch d.kind {
	case "eval-policy-judge-model":
		j := evaluationJudge{Kind: r.id, CallMode: "separate", Prompt: m.data.EvaluationPrompt}
		if i >= 0 {
			j = p.Judges[i]
		}
		j.Kind = r.id
		j.Endpoint = ""
		switch {
		case strings.HasPrefix(r.id, "llm:"):
			j.Kind = "llm"
			j.Model = strings.TrimPrefix(r.id, "llm:")
		case r.id == "diffusion":
			j.Model = m.simString("monitor_local_model")
			j.Endpoint = "auto"
		default:
			j.Model = m.simString("monitor_model")
		}
		if i < 0 {
			j.Name = strings.TrimSuffix(strings.TrimSuffix(r.label, " (classifier)"), " (LLM)")
			p.Judges = []evaluationJudge{j}
		} else {
			p.Judges[i] = j
		}
		return m.savePolicyDraft(d, p)
	case "eval-policy-judge":
		switch r.id {
		case "model":
			return m.policyModelPicker(d)
		case "call_mode":
			m.dialog = &dialog{kind: "eval-policy-judge-call", title: "Call mode", parent: d, args: d.args, rows: []row{{id: "separate", label: "Separate", preview: "One request per enabled behavior. Each assessment has its own context."}, {id: "bundled", label: "Bundled", preview: "Assess all enabled behaviors in one request. Opens a warning before changing."}}}
			return nil
		case "prompt":
			m.editing = "policy-judge-prompt"
			m.editReturn = d
			m.dialog = nil
			m.editor.SetValue(explicitTemplate("policy-judge-prompt", p.Judges[i].Prompt))
			m.focus = 1
			m.reflow()
			return m.editor.Focus()
		}
	case "eval-policy-behavior-library":
		b := m.libraryBehavior(r.id)
		if b == nil {
			return nil
		}
		p.Behaviors = append(p.Behaviors, evaluationBehavior{Name: b.Name, Spec: b.Spec, Enabled: false, Expected: "present", Threshold: .8, SourceID: b.ID, SourceRevision: b.Revision})
		return m.savePolicyDraft(d, p)
	case "eval-policy-behaviors":
		if r.id == "library" {
			return m.openBehaviorLibraryPicker(d, "eval-policy-behavior-library", d.args)
		}
		if r.id == "new" {
			m.dialog = &dialog{kind: "eval-policy-behavior-new", title: "New behavior", parent: d, args: d.args}
			m.dialog.add("Name", "")
			return m.dialog.fields[0].input.Focus()
		}
		b, _ := strconv.Atoi(r.id)
		return child(m.openPolicyBehavior(id, b))
	case "eval-policy-behavior":
		if r.id == "library-save" {
			b := p.Behaviors[k]
			return m.openPolicyBehaviorLibrary(d, p.ID, b)
		}
		if r.id == "expected" {
			p.Behaviors[k].Expected = nextExpected(p.Behaviors[k].Expected)
			return m.savePolicyDraft(&dialog{parent: d}, p)
		}
		if r.id == "enabled" {
			p.Behaviors[k].Enabled = !p.Behaviors[k].Enabled
			return m.savePolicyDraft(&dialog{parent: d}, p)
		}
		if r.id == "spec" {
			m.editing = "policy-behavior-spec"
			m.editReturn = d
			m.dialog = nil
			m.editor.SetValue(p.Behaviors[k].Spec)
			m.focus = 1
			m.reflow()
			return m.editor.Focus()
		}
	case "eval-policy-judge-call":
		if r.id == "bundled" {
			m.dialog = &dialog{kind: "eval-policy-judge-bundled", title: "Use bundled calls?", parent: d.parent, args: d.args, rows: []row{{id: "cancel", label: "Keep Separate", preview: "One request per behavior."}, {id: "confirm", label: "Use Bundled", preview: "Fewer requests, but behaviors share one response and can affect each other’s scores. Compare results before relying on them."}}}
			return nil
		}
		p.Judges[i].CallMode = "separate"
		return m.savePolicyDraft(d, p)
	case "eval-policy-judge-bundled":
		if r.id == "cancel" {
			m.dialog = d.parent
			return nil
		}
		p.Judges[i].CallMode = "bundled"
		return m.savePolicyDraft(d, p)
	case "eval-policy-judge-delete", "eval-policy-behavior-delete":
		if r.id == "cancel" {
			m.dialog = d.parent
			return nil
		}
		if k >= 0 {
			p.Behaviors = append(p.Behaviors[:k], p.Behaviors[k+1:]...)
		} else {
			p.Judges = append(p.Judges[:i], p.Judges[i+1:]...)
		}
		return m.savePolicyDraft(&dialog{parent: d.parent.parent}, p)
	}
	if r.id == "delete" {
		m.dialog = &dialog{kind: d.kind + "-delete", title: "Remove? Past results remain", parent: d, args: d.args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "confirm", label: "Remove"}}}
		return nil
	}
	if k >= 0 && (r.id == "name" || r.id == "threshold") {
		args := policyBehaviorArgs(id, k)
		args["field"] = r.id
		kind := "eval-policy-behavior-field"
		value := p.Behaviors[k].Name
		if r.id == "threshold" {
			value = fmt.Sprint(p.Behaviors[k].Threshold * 100)
		}
		m.dialog = &dialog{kind: kind, title: r.label, parent: d, args: args}
		m.dialog.add(r.label, value)
		return m.dialog.fields[0].input.Focus()
	}
	return nil
}

// Find the policy root when replacing a child screen with a sibling list.
func policyConfigParent(d *dialog) *dialog {
	for p := d.parent; p != nil; p = p.parent {
		if p.kind == "eval-policy-config" {
			return p
		}
	}
	return nil
}
func (m *model) savePolicyEditor(text string) tea.Cmd {
	d := m.editReturn
	id := d.args["id"].(string)
	p := m.findEvaluationPolicy(id)
	if p == nil {
		return nil
	}
	raw, _ := json.Marshal(p)
	var draft evaluationPolicy
	_ = json.Unmarshal(raw, &draft)
	i, _ := d.args["judge"].(int)
	k := -1
	if value, ok := d.args["behavior"].(int); ok {
		k = value
	}
	if strings.TrimSpace(text) == "" {
		m.status = "Write the criteria or prompt before saving"
		return nil
	}
	if m.editing == "policy-judge-prompt" && (i < 0 || i >= len(draft.Judges) || (d.args["judge_id"] != nil && d.args["judge_id"] != draft.Judges[i].ID)) {
		m.status = "Judge no longer exists; cancel this draft"
		return nil
	}
	if m.editing == "policy-behavior-spec" && (k < 0 || k >= len(draft.Behaviors)) {
		m.status = "Behavior no longer exists; cancel this draft"
		return nil
	}
	switch m.editing {
	case "policy-judge-prompt":
		draft.Judges[i].Prompt = text
	case "policy-behavior-new":
		draft.Behaviors = append(draft.Behaviors, evaluationBehavior{Name: d.fields[0].input.Value(), Spec: text, Threshold: 0.8, Enabled: true})
	default:
		draft.Behaviors[k].Spec = text
	}
	return m.submitEditor("evaluation.policy.save", map[string]any{"id": draft.ID, "name": draft.Name, "judges": draft.Judges, "behaviors": draft.Behaviors, "actions": draft.Actions})
}
