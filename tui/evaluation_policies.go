package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

// Policies are reusable assessment configurations; collections only own data.
type evaluationPolicy struct {
	ID, Name  string
	Judges    []evaluationJudge
	Behaviors []evaluationBehavior
	Revision  int
	Actions   evaluationActions `json:"actions"`
}
type evaluationActions struct {
	TrainOnPass bool `json:"train_on_pass"`
}
type evaluationProgress struct {
	Run, Record, Title, Judge, Stage, Text string
	Call, Total                            int
}

type evaluationRun struct {
	Opened, Live                    bool
	Progress                        evaluationProgress `json:"progress"`
	ID, Created, Collection, Status string
	CollectionName                  string `json:"collection_name"`
	Policy                          struct {
		ID, Name  string
		Revision  int
		Judges    []historicalEvaluationJudge
		Behaviors []evaluationBehavior
	}
	Items, Records   []string
	Count, Completed int
	Passed           *bool
	Results          []evaluationRecord
}

func (m *model) findEvaluationPolicy(id string) *evaluationPolicy {
	for i := range m.data.EvaluationPolicies {
		p := &m.data.EvaluationPolicies[i]
		if p.ID == id || p.Name == id {
			return p
		}
	}
	return nil
}
func (m *model) activePolicyName() string {
	if p := m.findEvaluationPolicy(m.data.ActiveEvaluationPolicy); p != nil {
		return p.Name
	}
	return "Choose a policy"
}
func (m *model) evaluationAreaRows() []row {
	rows := []row{{id: "back", kind: "eval-back", label: "← Evaluate"}}
	switch m.evalArea {
	case "policies":
		rows = append(rows, row{id: "new", kind: "eval-policy-new", label: "+ New policy", preview: "Choose behaviors and a judge to assess complete documents or conversations."})
		for _, p := range m.data.EvaluationPolicies {
			label := p.Name
			if p.ID == m.data.ActiveEvaluationPolicy {
				label += " · active"
			}
			rows = append(rows, row{id: p.ID, kind: "eval-policy", label: label, preview: fmt.Sprintf("%d behaviors · revision %d. ENTER to configure; set active to use with /eval.", len(p.Behaviors), p.Revision)})
		}
	case "runs":
		for i := len(m.data.EvaluationRuns) - 1; i >= 0; i-- {
			r := m.data.EvaluationRuns[i]
			rows = append(rows, row{id: r.ID, kind: "eval-run", label: fmt.Sprintf("%d · %s · %s", i+1, r.Policy.Name, evaluationStatus(evaluationSummary{Status: r.Status, Passed: r.Passed})), preview: fmt.Sprintf("%s · %d/%d assessed · policy revision %d", r.Created, r.Completed, r.Count, r.Policy.Revision)})
		}
	default:
		return []row{{id: "data", kind: "eval-area", label: "Data", preview: "Documents and conversation traces to assess."}, {id: "policies", kind: "eval-area", label: "Policies", preview: "Configure behaviors and one judge per policy for complete traces. Active: " + m.activePolicyName()}, {id: "runs", kind: "eval-area", label: "Runs", preview: "Results of applying policies to data, with frozen inputs and configuration."}, {id: "new-run", kind: "eval-new-run", label: "+ New run", preview: "Choose a dataset, policy and options for one evaluation run."}}
	}
	return rows
}
func (m *model) openEvaluationPolicies() tea.Cmd {
	d := &dialog{kind: "eval-policy-list", title: "Policies · active: " + m.activePolicyName()}
	for _, p := range m.data.EvaluationPolicies {
		label := p.Name
		if p.ID == m.data.ActiveEvaluationPolicy {
			label += " · active"
		}
		d.rows = append(d.rows, row{id: p.ID, label: label, preview: fmt.Sprintf("%d behaviors. Open to edit or make active for /eval.", len(p.Behaviors))})
	}
	d.rows = append(d.rows, row{id: "new", label: "+ New policy", preview: "Group behavior assessments into a reusable policy."})
	m.dialog = d
	return nil
}
func (m *model) newEvaluationPolicy(parent *dialog) tea.Cmd {
	m.dialog = &dialog{kind: "eval-policy-new", title: "New policy", parent: parent}
	m.dialog.add("Name", "")
	return m.dialog.fields[0].input.Focus()
}
func (m *model) openEvaluationPolicy(id string) tea.Cmd {
	p := m.findEvaluationPolicy(id)
	if p == nil {
		return nil
	}
	active := "Make active"
	if p.ID == m.data.ActiveEvaluationPolicy {
		active = "Active policy"
	}
	judgeLabel := "Judge · Choose model"
	if len(p.Judges) == 1 {
		judgeLabel = "Judge · " + m.evaluatorModelName(evaluator{Kind: p.Judges[0].Kind, Model: p.Judges[0].Model})
	}
	onPass := "Record only"
	if p.Actions.TrainOnPass {
		onPass = "Mark for training"
	}
	m.dialog = &dialog{kind: "eval-policy-config", title: p.Name, args: map[string]any{"id": p.ID}, rows: []row{
		{id: "name", label: "Name · " + p.Name, preview: "Name used by /eval and /loom --eval."},
		{id: "behaviors", label: fmt.Sprintf("Behaviors · %d", len(p.Behaviors)), preview: "The judge receives the complete saved document or conversation. All enabled behaviors must pass. Inputs that exceed context must fail visibly."},
		{id: "judges", label: judgeLabel, preview: "Configure one LLM, its prompt template and call settings for this policy’s behaviors."},
		{id: "on-pass", label: "On pass · " + onPass, preview: "Always record results. Optionally mark passing items for training. /eval --train-on-pass true|false overrides this for one run."},
		{id: "active", label: active, preview: "Use this policy when /eval has no explicit policy name."},
		{id: "delete", label: "Remove policy…", preview: "Remove this configuration; historical results remain."}}}
	return nil
}
func (m *model) submitEvaluationPolicy(d *dialog) tea.Cmd {
	if strings.HasPrefix(d.kind, "eval-policy-judge") || strings.HasPrefix(d.kind, "eval-policy-behavior") {
		return m.submitPolicyJudge(d)
	}
	id, _ := d.args["id"].(string)
	p := m.findEvaluationPolicy(id)
	if d.kind == "eval-policy-new" {
		name := strings.TrimSpace(d.fields[0].input.Value())
		if name == "" {
			m.status = "Name the policy"
			return nil
		}
		cmd := m.saveDialog(d, "evaluation.policy.save", map[string]any{"name": name, "judges": []evaluationJudge{}, "behaviors": []evaluationBehavior{}})
		if cmd != nil {
			m.evalPolicyCreating = true
		}
		return cmd
	}
	if p == nil && d.kind != "eval-policy-list" {
		return nil
	}
	if d.kind == "eval-policy-name" {
		return m.saveDialog(d, "evaluation.policy.save", map[string]any{"id": p.ID, "name": d.fields[0].input.Value(), "judges": p.Judges, "behaviors": p.Behaviors, "actions": p.Actions})
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	if d.kind == "eval-policy-list" {
		if r.id == "new" {
			return m.newEvaluationPolicy(d)
		}
		cmd := m.openEvaluationPolicy(r.id)
		m.dialog.parent = d
		return cmd
	}
	if d.kind == "eval-policy-delete" {
		if r.id == "delete" {
			m.dialog = d.parent.parent
			return m.send("evaluation.policy.delete", map[string]any{"id": p.ID})
		}
		m.dialog = d.parent
		return nil
	}
	switch r.id {
	case "on-pass":
		draft := *p
		draft.Actions.TrainOnPass = !draft.Actions.TrainOnPass
		return m.savePolicyDraft(&dialog{parent: d}, draft)
	case "name":
		m.dialog = &dialog{kind: "eval-policy-name", title: "Policy name", parent: d, args: d.args}
		m.dialog.add("Name", p.Name)
		return m.dialog.fields[0].input.Focus()
	case "active":
		return m.saveDialog(d, "evaluation.policy.active", map[string]any{"policy": p.ID})
	case "behaviors":
		m.openPolicyBehaviors(p.ID)
		m.dialog.parent = d
	case "judges":
		m.openEvaluationJudge(p.ID)
		m.dialog.parent = d
	case "delete":
		m.dialog = &dialog{kind: "eval-policy-delete", title: "Remove policy? Results remain", parent: d, args: d.args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "delete", label: "Remove policy"}}}
	}
	return nil
}
func (m *model) evaluatorModelName(e evaluator) string {
	if e.Kind == "llm" {
		for _, model := range m.data.SelectorModels {
			if model.Alias == e.Model {
				return model.Name + " (LLM)"
			}
		}
		return e.Model + " (LLM)"
	}
	if e.Kind == "jev" {
		return "Jev (classifier)"
	}
	if e.Kind == "diffusion" {
		return "DiffusionGemma (classifier)"
	}
	return e.Model + " (classifier)"
}
func (m *model) evaluationRunView(width int) string {
	if m.evalRun == nil || m.evalRun.ID != m.targetRow().id {
		return "Runs\n\nChoose a run and press ENTER to read its results.\nEach run preserves the exact data and policy used."
	}
	r := m.evalRun
	text := fmt.Sprintf("%s · %s · %d/%d assessed\n", r.Policy.Name, r.Status, r.Completed, r.Count)
	if r.Status == "running" {
		p := r.Progress
		text += p.Stage
		if p.Total > 0 {
			text += fmt.Sprintf(" · call %d/%d", p.Call, p.Total)
		}
		text += " · " + p.Judge + "\n"
		if p.Text != "" {
			text += p.Text + "\n"
		}
		text += "\nItem · " + p.Title + "\n"
	}
	text += fmt.Sprintf("%s · policy revision %d\n", r.Created, r.Policy.Revision)
	if r.CollectionName != "" {
		text += "Dataset · " + r.CollectionName + "\n"
	}
	// A run stores one result per behavior. Render all judgments beside one
	// copy of each frozen input rather than repeating the document for each.
	groups := []evaluationRecord{}
	indices := map[string]int{}
	for _, result := range r.Results {
		key := string(result.Source) + "\x00" + result.Title + "\x00" + result.Text
		i, ok := indices[key]
		if !ok {
			i = len(groups)
			indices[key] = i
			item := result
			item.Judgments = nil
			passed := true
			item.Passed = &passed
			groups = append(groups, item)
		}
		groups[i].Judgments = append(groups[i].Judgments, result)
		priority := map[string]int{"complete": 0, "queued": 1, "running": 2, "stopped": 3, "interrupted": 3, "failed": 4}
		if priority[result.Status] > priority[groups[i].Status] {
			groups[i].Status = result.Status
		}
		if !result.Result.Passed {
			*groups[i].Passed = false
		}
	}
	for _, item := range groups {
		text += "\n────────────────\n" + m.collectionItemView(&item, width) + "\n"
	}

	return ansi.Wrap(safe(text), width, "")
}

// Refresh only when the backend summary changed, preserving scroll while live.
func (m *model) evaluationRunStale(id string) bool {
	if m.evalRun == nil || m.evalRun.ID != id {
		return true
	}
	for _, r := range m.data.EvaluationRuns {
		if r.ID == id {
			return r.Status != m.evalRun.Status || r.Completed != m.evalRun.Completed || len(r.Records) != len(m.evalRun.Records)
		}
	}
	return false
}
