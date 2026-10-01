package main

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type evaluationCollection struct {
	ID, Name string
	Judges   []string
	Items    []evaluationSummary
}

func (m *model) currentEvaluation() *evaluationCollection {
	for i := range m.data.EvaluationSets {
		if m.data.EvaluationSets[i].ID == m.evalCollection {
			return &m.data.EvaluationSets[i]
		}
	}
	return nil
}
func (m *model) collectionItems() []evaluationSummary {
	if c := m.currentEvaluation(); c != nil {
		return c.Items
	}
	return nil
}
func (m *model) collectionRows() []row {
	if c := m.currentEvaluation(); c != nil {
		filter := m.evalFilter
		if filter == "" {
			filter = "all"
		}
		rows := []row{{id: "back", kind: "eval-back", label: "← Data"}, {id: "config", kind: "eval-config", label: "Collection settings", preview: "Rename this data collection or make it the default destination. Assessment behaviors belong in Policies."}, {id: "add", kind: "eval-add", label: "+ Add data", preview: "Choose saved documents, conversations or existing judgments. Adding data does not run a model."}, {id: "run", kind: "eval-execute", label: "Run · " + m.activePolicyName(), preview: "Assess selected or pending data using this policy. /policy changes the active policy."}, {id: "filter", kind: "eval-filter", label: "Show · " + filter}}
		for i := len(c.Items) - 1; i >= 0; i-- {
			e := c.Items[i]
			status := evaluationStatus(e)
			if filter == "training" && !e.Training || filter == "pass" && status != "PASS" || filter == "fail" && status != "FAIL" || filter == "unfinished" && e.Status == "complete" {
				continue
			}
			mark := "  "
			if m.evalSelection[e.ID] {
				mark = "✓ "
			}
			train := ""
			if e.Training {
				train = " ★"
			}
			rows = append(rows, row{id: e.ID, kind: "evaluation", label: mark + status + train + " · " + e.Title, preview: e.Kind})
		}
		return rows
	}
	if m.evalArea != "data" {
		return m.evaluationAreaRows()
	}
	rows := []row{{id: "back", kind: "eval-back", label: "← Evaluate"}, {id: "new", kind: "eval-create", label: "+ New collection", preview: "Collect saved documents and conversation traces. Adding data does not run a model."}}
	for _, c := range m.data.EvaluationSets {
		pass, fail := 0, 0
		for _, i := range c.Items {
			if i.Passed != nil {
				if *i.Passed {
					pass++
				} else {
					fail++
				}
			}
		}
		active := ""
		if c.ID == m.data.ActiveEvaluation {
			active = " · active"
		}
		rows = append(rows, row{id: c.ID, kind: "eval-collection", label: c.Name + active, preview: fmt.Sprintf("%d items · %d pass · %d fail · %d pending/evidence", len(c.Items), pass, fail, len(c.Items)-pass-fail)})
	}
	return rows
}
func (m *model) openCollectionConfig() tea.Cmd {
	c := m.currentEvaluation()
	if c == nil {
		m.status = "Open a data collection first"
		return nil
	}
	active := "Use as default"
	if c.ID == m.data.ActiveEvaluation {
		active = "Default collection"
	}
	m.dialog = &dialog{kind: "eval-collection-config", title: c.Name, rows: []row{{id: "name", label: "Name · " + c.Name, preview: "Rename this collection of data."}, {id: "active", label: active, preview: "Use this collection for data added by /eval from documents and conversations."}}}
	return nil
}
func (m *model) enterCollection(id string) {
	m.evalCollection = id
	m.selected = 0
	m.focus = 0
	m.evalSelection = map[string]bool{}
	m.evalFilter = ""
	m.filter = ""
	m.evaluation = nil
	m.reflow()
}
func (m *model) evaluationCollectionAction(kind, id string) tea.Cmd {
	switch kind {
	case "eval-collection":
		m.enterCollection(id)
	case "eval-area":
		m.evalArea = id
		m.enterCollection("")
	case "eval-policy":
		return m.openEvaluationPolicy(id)
	case "eval-policy-new":
		return m.newEvaluationPolicy(nil)
	case "eval-run":
		m.focus = 1
		return m.send("evaluation.run.open", map[string]any{"id": id})
	case "eval-back":
		if m.evalCollection == "" {
			m.evalArea = ""
		}
		m.enterCollection("")
	case "eval-create":
		m.dialog = &dialog{kind: "eval-collection-new", title: "New collection"}
		m.dialog.add("Name", "")
		return m.dialog.fields[0].input.Focus()
	case "eval-config":
		return m.openCollectionConfig()
	case "eval-add":
		return m.openCollectionItems()
	case "eval-execute":
		return m.openEval("/eval")
	}
	return nil
}
func (m *model) openCollectionItems() tea.Cmd {
	d := &dialog{kind: "eval-add-items", title: "Add items · existing evidence is retained; no judging", args: map[string]any{"selected": map[string]bool{}, "targets": map[string]map[string]any{}}}
	targets := d.args["targets"].(map[string]map[string]any)
	add := func(id, label string, target map[string]any) {
		d.rows = append(d.rows, row{id: id, label: label})
		targets[id] = target
	}
	for _, n := range m.data.Nodes {
		add("node:"+n.ID, "Document · "+documentLabel(n), map[string]any{"node": n.ID})
	}
	for _, run := range m.data.SimulationRuns {
		for i := 0; i < run.Count; i++ {
			c := simulationConversation{Index: i}
			if i < len(run.Conversations) {
				c = run.Conversations[i]
			}
			add(conversationKey(run.ID, i), conversationName(c, false), map[string]any{"run": run.ID, "conversation": i})
		}
	}
	for _, e := range m.data.Evaluations {
		if e.Status == "complete" {
			add("judgment:"+e.ID, "Judged · "+e.Title+" · "+e.Evaluator, map[string]any{"evaluation": e.ID})
		}
	}
	m.dialog = d
	return nil
}
func (m *model) submitCollection(d *dialog) tea.Cmd {
	c := m.currentEvaluation()
	if d.kind == "eval-collection-new" {
		name := strings.TrimSpace(d.fields[0].input.Value())
		if name == "" {
			m.status = "Name the collection"
			return nil
		}
		cmd := m.send("evaluation.collection.save", map[string]any{"name": name})
		if cmd != nil {
			m.status = "Collection created"
			m.evalCreating = true
			m.dialog = nil
		}
		return cmd
	}
	if c == nil {
		return nil
	}
	switch d.kind {
	case "eval-collection-name":
		return m.saveDialog(d, "evaluation.collection.save", map[string]any{"id": c.ID, "name": d.fields[0].input.Value()})
	case "eval-add-items":
		selected := d.args["selected"].(map[string]bool)
		targets := []map[string]any{}
		all := d.args["targets"].(map[string]map[string]any)
		for _, r := range append(append([]row{}, d.rows...), d.allRows...) {
			if selected[r.id] {
				targets = append(targets, all[r.id])
				delete(selected, r.id)
			}
		}
		cmd := m.send("evaluation.collection.add", map[string]any{"collection": c.ID, "targets": targets, "attach_evidence": true})
		if cmd != nil {
			m.dialog = nil
		}
		return cmd
	case "eval-collection-config":
		if len(d.rows) == 0 {
			return nil
		}
		switch d.rows[d.index].id {
		case "workspaces", "keys":
			cmd := m.perform(d.rows[d.index].id)
			if m.dialog != nil {
				m.dialog.parent = d
			}
			return cmd
		case "name":
			m.dialog = &dialog{kind: "eval-collection-name", title: "Collection name", parent: d}
			m.dialog.add("Name", c.Name)
			return m.dialog.fields[0].input.Focus()
		case "active":
			return m.send("evaluation.collection.active", map[string]any{"collection": c.ID})

		}
	}
	return nil
}

func (m *model) collectionItemView(e *evaluationRecord, width int) string {
	text := strings.ToUpper(evaluationStatus(evaluationSummary{Status: e.Status, Passed: e.Passed})) + " · " + e.Title + "\n"
	if e.Training {
		text += "★ Training\n"
	}
	if len(e.GenerationModels) > 0 {
		text += "Generated by: " + strings.Join(e.GenerationModels, " · ") + "\n"
	}
	for _, j := range e.Judgments {
		status := strings.ToUpper(j.Status)
		if j.Status == "complete" {
			status = "FAIL"
			if j.Result.Passed {
				status = "PASS"
			}
		}
		text += fmt.Sprintf("\n%s · %s · revision %d\nJudge: %s\nCriteria: %s\n", status, j.Definition.Name, j.Definition.Revision, j.Definition.Model, j.Definition.Spec)
		if j.Result.Probability != nil {
			text += fmt.Sprintf("Probability: %.1f%%\n", *j.Result.Probability*100)
		}
		text += j.Result.Reason + "\n" + j.Result.Evidence + "\n" + j.Error
	}
	for _, evidence := range e.Evidence {
		var pretty bytes.Buffer
		json.Indent(&pretty, evidence, "", "  ")
		text += "\nAttached policy evidence · original scope (not a whole-item grade)\n" + pretty.String() + "\n"
	}
	if e.Note != "" {
		text += "\nNotes\n" + e.Note + "\n"
	}
	text += "\nSaved text\n\n" + e.Text
	return ansi.Wrap(safe(text), width, "")
}
