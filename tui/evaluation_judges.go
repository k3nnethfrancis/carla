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
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Kind      string               `json:"kind"`
	Model     string               `json:"model"`
	Prompt    string               `json:"prompt"`
	Endpoint  string               `json:"endpoint,omitempty"`
	Revision  int                  `json:"revision"`
	Missing   bool                 `json:"missing,omitempty"`
	CallMode  string               `json:"call_mode"`
	Behaviors []evaluationBehavior `json:"behaviors"`
}
type evaluationBehavior struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Spec           string  `json:"spec"`
	Threshold      float64 `json:"threshold"`
	Enabled        bool    `json:"enabled"`
	Revision       int     `json:"revision"`
	SourceID       string  `json:"source_id,omitempty"`
	SourceRevision int     `json:"source_revision,omitempty"`
}

// Legacy frozen runs contained a flat evaluator. Adapt only the display shape.
func (j *evaluationJudge) UnmarshalJSON(data []byte) error {
	type wire evaluationJudge
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*j = evaluationJudge(w)
	if j.Behaviors == nil {
		var old evaluator
		if err := json.Unmarshal(data, &old); err != nil {
			return err
		}
		j.Behaviors = []evaluationBehavior{{ID: old.ID, Name: old.Name, Spec: old.Spec, Threshold: old.Threshold, Enabled: true, Revision: old.Revision}}
	}
	return nil
}
func policyJudgeArgs(id string, j, b int) map[string]any {
	return map[string]any{"id": id, "judge": j, "behavior": b}
}
func (m *model) openPolicyJudges(id string) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil {
		return nil
	}
	d := &dialog{kind: "eval-policy-judges", title: p.Name + " · Judges", args: policyJudgeArgs(id, -1, -1)}
	for i, j := range p.Judges {
		label := m.evaluatorModelName(evaluator{Kind: j.Kind, Model: j.Model})
		if j.Missing {
			label = "Missing judge · " + j.Name
		} else if j.Name != "" {
			label += " · " + j.Name
		}
		d.rows = append(d.rows, row{id: strconv.Itoa(i), label: label, preview: fmt.Sprintf("%s · %d behaviors. Open model, call settings and behaviors.", m.evaluatorModelName(evaluator{Kind: j.Kind, Model: j.Model}), len(j.Behaviors))})
	}
	d.rows = append(d.rows, row{id: "new", label: "+ New judge", preview: "Choose a model, then add the behaviors it should assess."})
	m.dialog = d
	return nil
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
	rows := []row{{id: "name", label: "Name · " + j.Name, preview: "Name this judge within the policy."}, {id: "model", label: "Model · " + m.evaluatorModelName(evaluator{Kind: j.Kind, Model: j.Model}), preview: "Classifiers return probabilities; LLMs return structured judgments."}, {id: "call_mode", label: "Call mode · " + mode, preview: "Separate assesses each behavior in its own request. Bundled assesses them together."}, {id: "behaviors", label: fmt.Sprintf("Behaviors · %d", len(j.Behaviors)), preview: "Define criteria and detection rules. Disabled behaviors are skipped."}}
	if j.Kind == "llm" {
		rows = append(rows, row{id: "prompt", label: "Prompt", preview: "Instructions used by this judge to assess behaviors. Open to read or edit the full prompt."})
	}
	rows = append(rows, row{id: "delete", label: "Remove judge…", preview: "Remove this judge and its behaviors from this policy. Past results remain."})
	m.dialog = &dialog{kind: "eval-policy-judge", title: j.Name, args: policyJudgeArgs(id, i, -1), rows: rows}
	return nil
}
func (m *model) openPolicyBehaviors(id string, i int) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil || i < 0 || i >= len(p.Judges) {
		return nil
	}
	j := p.Judges[i]
	d := &dialog{kind: "eval-policy-behaviors", title: j.Name + " · Behaviors", args: policyJudgeArgs(id, i, -1)}
	for k, b := range j.Behaviors {
		label := b.Name
		if !b.Enabled {
			label += " · off"
		}
		d.rows = append(d.rows, row{id: strconv.Itoa(k), label: label, preview: b.Spec})
	}
	d.rows = append(d.rows, row{id: "library", label: "From library", preview: "Copy an existing behavior spec into this judge. Local settings stay here."})
	d.rows = append(d.rows, row{id: "new", label: "+ New behavior", preview: "Name the behavior, then write the criteria this judge should assess."})
	m.dialog = d
	return nil
}
func (m *model) openPolicyBehavior(id string, i, k int) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil || i < 0 || i >= len(p.Judges) || k < 0 || k >= len(p.Judges[i].Behaviors) {
		return nil
	}
	j := p.Judges[i]
	b := j.Behaviors[k]
	enabled := "Off"
	if b.Enabled {
		enabled = "On"
	}
	rows := []row{{id: "enabled", label: "Enabled · " + enabled, preview: "Off skips this behavior while preserving its settings."}, {id: "name", label: "Name · " + b.Name, preview: "Short name shown beside this behavior’s results."}, {id: "spec", label: "Behavior spec", preview: "Criteria this judge assesses. " + b.Spec}}
	if j.Kind != "llm" {
		rows = append(rows, row{id: "threshold", label: fmt.Sprintf("Pass threshold · %.0f%%", b.Threshold*100), preview: "Pass when the classifier’s probability that these criteria are met reaches this threshold."})
	}
	rows = append(rows, row{id: "library-save", label: "Save spec to library", preview: "Create a reusable spec from this behavior. Model, threshold and enabled state stay in this judge."})
	rows = append(rows, row{id: "delete", label: "Remove behavior…", preview: "Remove this behavior from the judge. Previous results keep their exact spec."})
	m.dialog = &dialog{kind: "eval-policy-behavior", title: b.Name, args: policyJudgeArgs(id, i, k), rows: rows}
	return nil
}
func (m *model) policyModelPicker(d *dialog) tea.Cmd {
	n := &dialog{kind: "eval-policy-judge-model", title: "Model", parent: d, args: d.args}
	n.rows = []row{{id: "diffusion", label: "DiffusionGemma (classifier)", preview: "Local OpenJev classification; no API key."}, {id: "jev", label: "Jev (classifier)", preview: "Hosted classification via OpenRouter; requires an API key."}}
	for _, model := range m.data.SelectorModels {
		n.rows = append(n.rows, row{id: "llm:" + model.Alias, label: model.Name + " (LLM)", preview: "Local instruction model returning structured judgments."})
	}
	m.dialog = n
	return nil
}
func (m *model) savePolicyDraft(d *dialog, p evaluationPolicy) tea.Cmd {
	return m.saveDialog(d, "evaluation.policy.save", map[string]any{"id": p.ID, "name": p.Name, "judges": p.Judges, "actions": p.Actions})
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
	i, _ := d.args["judge"].(int)
	k, _ := d.args["behavior"].(int)
	if d.kind == "eval-policy-judge-field" || d.kind == "eval-policy-behavior-field" {
		value := strings.TrimSpace(d.fields[0].input.Value())
		if value == "" {
			m.status = "Enter a value"
			return nil
		}
		field := d.args["field"].(string)
		if d.kind == "eval-policy-judge-field" {
			p.Judges[i].Name = value
		} else if field == "threshold" {
			v, err := strconv.ParseFloat(value, 64)
			if err != nil || v <= 0 || v > 100 {
				m.status = "Enter a percentage greater than 0 and at most 100"
				return nil
			}
			p.Judges[i].Behaviors[k].Threshold = v / 100
		} else {
			p.Judges[i].Behaviors[k].Name = value
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
	case "eval-policy-judges":
		if r.id == "new" {
			return m.policyModelPicker(d)
		}
		j, _ := strconv.Atoi(r.id)
		return child(m.openPolicyJudge(id, j))
	case "eval-policy-judge-model":
		j := evaluationJudge{Kind: r.id, CallMode: "separate", Behaviors: []evaluationBehavior{}, Prompt: m.data.EvaluationPrompt}
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
			p.Judges = append(p.Judges, j)
		} else {
			p.Judges[i] = j
		}
		return m.savePolicyDraft(d, p)
	case "eval-policy-judge":
		switch r.id {
		case "behaviors":
			return child(m.openPolicyBehaviors(id, i))
		case "model":
			return m.policyModelPicker(d)
		case "call_mode":
			m.dialog = &dialog{kind: "eval-policy-judge-call", title: "Call mode", parent: d, args: d.args, rows: []row{{id: "separate", label: "Separate", preview: "One request per enabled behavior. Each assessment has its own context."}, {id: "bundled", label: "Bundled", preview: "Assess all enabled behaviors in one request. Opens a warning before changing."}}}
			return nil
		case "prompt":
			m.editing = "policy-judge-prompt"
			m.editReturn = d
			m.dialog = nil
			m.editor.SetValue(p.Judges[i].Prompt)
			m.focus = 1
			m.reflow()
			return m.editor.Focus()
		}
	case "eval-policy-behavior-library":
		b := m.libraryBehavior(r.id)
		if b == nil {
			return nil
		}
		p.Judges[i].Behaviors = append(p.Judges[i].Behaviors, evaluationBehavior{Name: b.Name, Spec: b.Spec, Enabled: true, Threshold: .8, SourceID: b.ID, SourceRevision: b.Revision})
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
		return child(m.openPolicyBehavior(id, i, b))
	case "eval-policy-behavior":
		if r.id == "library-save" {
			b := p.Judges[i].Behaviors[k]
			return m.send("behavior.save", map[string]any{"name": b.Name, "spec": b.Spec})
		}
		if r.id == "enabled" {
			p.Judges[i].Behaviors[k].Enabled = !p.Judges[i].Behaviors[k].Enabled
			return m.savePolicyDraft(&dialog{parent: d}, p)
		}
		if r.id == "spec" {
			m.editing = "policy-behavior-spec"
			m.editReturn = d
			m.dialog = nil
			m.editor.SetValue(p.Judges[i].Behaviors[k].Spec)
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
			p.Judges[i].Behaviors = append(p.Judges[i].Behaviors[:k], p.Judges[i].Behaviors[k+1:]...)
		} else {
			p.Judges = append(p.Judges[:i], p.Judges[i+1:]...)
		}
		return m.savePolicyDraft(&dialog{parent: d.parent.parent}, p)
	}
	if r.id == "delete" {
		m.dialog = &dialog{kind: d.kind + "-delete", title: "Remove? Past results remain", parent: d, args: d.args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "confirm", label: "Remove"}}}
		return nil
	}
	if r.id == "name" || r.id == "threshold" {
		args := policyJudgeArgs(id, i, k)
		args["field"] = r.id
		kind := "eval-policy-judge-field"
		value := p.Judges[i].Name
		if k >= 0 {
			kind = "eval-policy-behavior-field"
			value = p.Judges[i].Behaviors[k].Name
			if r.id == "threshold" {
				value = fmt.Sprint(p.Judges[i].Behaviors[k].Threshold * 100)
			}
		}
		m.dialog = &dialog{kind: kind, title: r.label, parent: d, args: args}
		m.dialog.add(r.label, value)
		return m.dialog.fields[0].input.Focus()
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
	i := d.args["judge"].(int)
	k, _ := d.args["behavior"].(int)
	if strings.TrimSpace(text) == "" {
		m.status = "Write the criteria or prompt before saving"
		return nil
	}
	switch m.editing {
	case "policy-judge-prompt":
		draft.Judges[i].Prompt = text
	case "policy-behavior-new":
		draft.Judges[i].Behaviors = append(draft.Judges[i].Behaviors, evaluationBehavior{Name: d.fields[0].input.Value(), Spec: text, Threshold: 0.8, Enabled: true})
	default:
		draft.Judges[i].Behaviors[k].Spec = text
	}
	return m.submitEditor("evaluation.policy.save", map[string]any{"id": draft.ID, "name": draft.Name, "judges": draft.Judges, "actions": draft.Actions})
}
