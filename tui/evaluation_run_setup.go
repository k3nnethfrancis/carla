package main

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
)

// Run setup is a draft. Choosing a dataset or policy never changes workspace
// defaults; Start sends the same evaluation command used outside Evaluate.
type evaluationRunDraft struct {
	Dataset, Policy string
	Selected        []string
	UseSelected     bool
	TrainOnPass     *bool // nil inherits the chosen policy's setting
}

func evaluationAreaTitle(area string) string {
	if area == "runs" {
		return "Results"
	} // Keep saved navigation compatible.
	return strings.Title(area)
}

func (m *model) evaluationDataset(id string) *evaluationCollection {
	for i := range m.data.EvaluationSets {
		if m.data.EvaluationSets[i].ID == id {
			return &m.data.EvaluationSets[i]
		}
	}
	return nil
}

func (m *model) newEvaluationRun(policy string, train *bool) tea.Cmd {
	if policy == "" {
		policy = m.data.ActiveEvaluationPolicy
	}
	if p := m.findEvaluationPolicy(policy); p != nil {
		policy = p.ID
	} else if policy != "" {
		m.status = "Choose an existing Evals policy"
		return nil
	}
	draft := &evaluationRunDraft{Dataset: m.data.ActiveEvaluation, Policy: policy, TrainOnPass: train}
	if c := m.currentEvaluation(); c != nil {
		draft.Dataset = c.ID
		draft.Selected = m.evaluationIDs()
		draft.UseSelected = len(draft.Selected) > 0
	} else if m.targetRow().kind == "eval-collection" {
		draft.Dataset = m.targetRow().id
	}
	return m.openEvaluationRunSetup(draft, nil)
}

func (m *model) openEvaluationRunSetup(draft *evaluationRunDraft, parent *dialog) tea.Cmd {
	dataset, policy, count := "Choose dataset", "Choose policy", 0
	if c := m.evaluationDataset(draft.Dataset); c != nil {
		dataset, count = c.Name, len(c.Items)
	}
	onPass := "Record only"
	if p := m.findEvaluationPolicy(draft.Policy); p != nil {
		policy = p.Name
		if p.Actions.TrainOnPass {
			onPass = "Mark for training"
		}
	}
	if draft.TrainOnPass != nil {
		onPass = "Record only"
		if *draft.TrainOnPass {
			onPass = "Mark for training"
		}
	} else {
		onPass = "Policy default · " + onPass
	}
	scope := fmt.Sprintf("All %d items", count)
	if draft.UseSelected {
		scope = fmt.Sprintf("Selected %d items", len(draft.Selected))
	}
	m.dialog = &dialog{kind: "eval-run-config", title: "New evaluation run", parent: parent, args: map[string]any{"draft": draft}, rows: []row{
		{id: "dataset", label: "Dataset · " + dataset, preview: "Saved documents and conversation traces to assess. Changing this does not change your default dataset."},
		{id: "policy", label: "Policy · " + policy, preview: "Use an existing Evals policy. Edit reusable judges and behaviors through /policy → Evals."},
		{id: "scope", label: "Data · " + scope, preview: "All items includes previously evaluated data. To assess a subset, select items in Data with SPACE, then use /eval."},
		{id: "training", label: "On pass · " + onPass, preview: "For this run only: record results, or also mark passing items for training. No model training is started."},
		{id: "start", label: "▶ Start run", preview: "Run the chosen policy on this data. Results preserve the inputs and policy used. ESC cancels setup."},
	}}
	return nil
}

func (m *model) submitEvaluationRun(d *dialog) tea.Cmd {
	if len(d.rows) == 0 || m.dialogRequest != "" {
		return nil
	}
	draft := d.args["draft"].(*evaluationRunDraft)
	id := d.rows[d.index].id
	if d.kind != "eval-run-config" {
		switch d.kind {
		case "eval-run-dataset":
			if draft.Dataset != id {
				draft.Selected, draft.UseSelected = nil, false
			}
			draft.Dataset = id
		case "eval-run-policy":
			draft.Policy = id
		case "eval-run-scope":
			draft.UseSelected = id == "selected"
		case "eval-run-training":
			draft.TrainOnPass = nil
			if id != "default" {
				value := id == "train"
				draft.TrainOnPass = &value
			}
		}
		parent := d.parent
		m.openEvaluationRunSetup(draft, parent.parent)
		for i, r := range m.dialog.rows {
			if r.id == strings.TrimPrefix(d.kind, "eval-run-") {
				m.dialog.index = i
			}
		}
		return nil
	}
	if id == "start" {
		c, p := m.evaluationDataset(draft.Dataset), m.findEvaluationPolicy(draft.Policy)
		if c == nil {
			d.args["error"] = "Choose a dataset first"
			return nil
		}
		if p == nil {
			d.args["error"] = "Choose a policy first"
			return nil
		}
		ids := []string{}
		if draft.UseSelected {
			ids = append(ids, draft.Selected...)
		} else {
			for _, item := range c.Items {
				ids = append(ids, item.ID)
			}
		}
		if len(ids) == 0 {
			d.args["error"] = "This dataset is empty. Add data before starting."
			return nil
		}
		args := map[string]any{"collection": c.ID, "policy": p.ID, "items": ids}
		if draft.TrainOnPass != nil {
			args["train_on_pass"] = *draft.TrainOnPass
		}
		return m.saveDialog(d, "evaluation.collection.run", args)
	}
	picker := &dialog{kind: "eval-run-" + id, title: "Choose " + id, parent: d, args: map[string]any{"draft": draft}}
	selected := ""
	switch id {
	case "dataset":
		selected = draft.Dataset
		for _, c := range m.data.EvaluationSets {
			picker.rows = append(picker.rows, row{id: c.ID, label: c.Name, preview: fmt.Sprintf("%d saved items", len(c.Items))})
		}
		if len(picker.rows) == 0 {
			d.args["error"] = "Create a dataset in Data and add items first"
			return nil
		}
	case "policy":
		selected = draft.Policy
		for _, p := range m.data.EvaluationPolicies {
			picker.rows = append(picker.rows, row{id: p.ID, label: p.Name, preview: fmt.Sprintf("%d judges · %d behaviors · revision %d", len(p.Judges), len(p.Behaviors), p.Revision)})
		}
		if len(picker.rows) == 0 {
			d.args["error"] = "Create a policy through /policy → Evals first"
			return nil
		}
	case "scope":
		picker.title = "Data to assess"
		picker.rows = []row{{id: "all", label: "All dataset items", preview: "Include previously evaluated items; each run records new results."}}
		if len(draft.Selected) > 0 {
			picker.rows = append(picker.rows, row{id: "selected", label: fmt.Sprintf("Selected %d items", len(draft.Selected))})
		}
		if draft.UseSelected {
			selected = "selected"
		}
	case "training":
		picker.title = "On pass · this run"
		picker.rows = []row{{id: "default", label: "Use policy default"}, {id: "record", label: "Record only"}, {id: "train", label: "Mark passing items for training"}}
		if draft.TrainOnPass != nil {
			selected = "record"
			if *draft.TrainOnPass {
				selected = "train"
			}
		}
	default:
		return nil
	}
	for i, r := range picker.rows {
		if r.id == selected {
			picker.index = i
		}
	}
	m.dialog = picker
	return nil
}
