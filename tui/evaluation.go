package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Definitions and result snapshots belong to Python; these structs only render
// them and carry explicit user actions back across the protocol.
type evaluator struct {
	ID, Name, Kind, Spec, Prompt, Model string
	Revision                            int
	Threshold                           float64
}
type evaluationSummary struct {
	ID, Title, Kind, Status, Created, Evaluator, Error string
	Passed                                             *bool
	Training                                           bool
	Revision                                           int
}
type evaluationRecord struct {
	GenerationModels                           []string `json:"generation_models"`
	ID, Title, Kind, Text, Status, Note, Error string
	Training                                   bool
	Definition                                 evaluator
	Result                                     struct {
		Passed           bool
		Reason, Evidence string
		Probability      *float64
	}
	Source json.RawMessage
	Trace  json.RawMessage
}

func (e evaluator) args() map[string]any {
	return map[string]any{"id": e.ID, "name": e.Name, "kind": e.Kind, "spec": e.Spec, "prompt": e.Prompt, "model": e.Model, "threshold": e.Threshold}
}
func (m *model) evaluator(id string) evaluator {
	for _, e := range m.data.Evaluators {
		if e.ID == id {
			return e
		}
	}
	return evaluator{}
}
func (m *model) openPolicy() tea.Cmd {
	m.dialog = &dialog{kind: "policy", title: "Policy", rows: []row{
		{id: "monitor", label: "Monitoring", preview: "Evaluate during generation; warn or explicitly stop."},
		{id: "selection", label: "Selection", preview: "Evaluate alternatives against criteria and choose which to continue."},
		{id: "evaluators", label: "Evaluations", preview: "Saved judges and criteria for /eval on whole documents or conversations."},
	}}
	return nil
}
func (m *model) openEvaluators() tea.Cmd {
	d := &dialog{kind: "eval-definitions", title: "Evaluations"}
	for _, e := range m.data.Evaluators {
		d.rows = append(d.rows, row{id: e.ID, label: e.Name + " · " + e.Kind, preview: e.Spec})
	}
	d.rows = append(d.rows, row{id: "new", label: "+ New evaluation"})
	m.dialog = d
	return nil
}
func (m *model) openEvaluator(id string) tea.Cmd {
	e := m.evaluator(id)
	rows := []row{{id: "name", label: e.Name, preview: "Rename evaluation"}, {id: "kind", label: "Judge · " + e.Kind}, {id: "model", label: "Model · " + e.Model}, {id: "spec", label: "Criteria", preview: e.Spec}}
	if e.Kind == "llm" {
		rows = append(rows, row{id: "prompt", label: "Judge prompt", preview: e.Prompt})
	} else {
		rows = append(rows, row{id: "threshold", label: fmt.Sprintf("Pass threshold · %.0f%%", e.Threshold*100), preview: "P(criteria met). Model estimate, not calibrated certainty."})
	}
	rows = append(rows, row{id: "delete", label: "Delete definition…", preview: "Historical results retain their frozen definition."})
	m.dialog = &dialog{kind: "eval-definition", title: fmt.Sprintf("%s · revision %d", e.Name, e.Revision), rows: rows, args: map[string]any{"id": id}}
	return nil
}
func (m *model) evaluationIDs() []string {
	var ids []string
	for _, e := range m.data.Evaluations {
		if m.evalSelection[e.ID] {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 && m.targetRow().kind == "evaluation" {
		ids = append(ids, m.targetRow().id)
	}
	return ids
}
func (m *model) evaluationTargets() []map[string]any {
	var targets []map[string]any
	switch m.section {
	case 1, 2:
		for _, id := range m.actionNodeIDs() {
			targets = append(targets, map[string]any{"node": id})
		}
	case 3:
		if args, ok := m.conversationTarget(); ok {
			return []map[string]any{args}
		}
		if r := m.targetRow(); r.kind == "simulation" {
			for _, run := range m.data.SimulationRuns {
				if run.ID == r.id {
					for i := 0; i < run.Count; i++ {
						targets = append(targets, map[string]any{"run": r.id, "conversation": i})
					}
				}
			}
		}
	case 4:
		for _, id := range m.evaluationIDs() {
			targets = append(targets, map[string]any{"evaluation": id})
		}
	}
	return targets
}
func parseEvalOptions(input string) (bool, error) {
	fields := strings.Fields(input)
	if len(fields) == 1 {
		return false, nil
	}
	if len(fields) == 3 && fields[1] == "--train-on-pass" && (fields[2] == "true" || fields[2] == "false") {
		return fields[2] == "true", nil
	}
	if len(fields) == 2 && (fields[1] == "--train-on-pass=true" || fields[1] == "--train-on-pass=false") {
		return strings.HasSuffix(fields[1], "=true"), nil
	}
	return false, fmt.Errorf("use /eval [--train-on-pass true|false]")
}
func (m *model) openEval(input string) tea.Cmd {
	auto, err := parseEvalOptions(input)
	if err != nil {
		m.status = "Error: " + err.Error()
		return nil
	}
	targets := m.evaluationTargets()
	if len(targets) == 0 {
		m.status = "Select saved documents or conversations to evaluate"
		return nil
	}
	if len(m.data.Evaluators) == 0 {
		m.status = "Create an evaluation in /policy, then run /eval"
		return m.openEvaluators()
	}
	d := &dialog{kind: "eval-run", title: fmt.Sprintf("%d selected · train on pass: %t", len(targets), auto), args: map[string]any{"targets": targets, "train_on_pass": auto}}
	for _, e := range m.data.Evaluators {
		d.rows = append(d.rows, row{id: e.ID, label: e.Name + " · " + e.Kind, preview: e.Spec})
	}
	m.dialog = d
	return nil
}
func evaluationStatus(e evaluationSummary) string {
	if e.Status == "failed" {
		return "ERROR"
	}
	if e.Status != "complete" {
		return e.Status
	}
	if e.Passed != nil && *e.Passed {
		return "PASS"
	}
	return "FAIL"
}
func (m *model) evaluationRows() []row {
	filter := m.evalFilter
	if filter == "" {
		filter = "all"
	}
	rows := []row{{id: "filter", kind: "eval-filter", label: "Filter · " + filter}, {id: "definitions", kind: "eval-definitions", label: "Evaluation definitions · /policy"}}
	for i := len(m.data.Evaluations) - 1; i >= 0; i-- {
		e := m.data.Evaluations[i]
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
		rows = append(rows, row{id: e.ID, kind: "evaluation", label: mark + status + train + " · " + e.Title, preview: e.Evaluator + " · " + e.Kind + " · " + e.Created})
	}
	return rows
}
func (m *model) evaluationView(width int) string {
	if m.evaluation == nil || m.evaluation.ID != m.targetRow().id {
		return "Evaluate saved documents or conversations with /eval.\n\nConfigure judges and criteria in /policy.\n\nSPACE selects items · ENTER actions\n/keep marks for training · /remove unmarks\n/snapshot exports marked items with metadata."
	}
	e := m.evaluation
	status := e.Status
	if status == "failed" {
		status = "ERROR"
	}
	if e.Status == "complete" {
		status = "FAIL"
		if e.Result.Passed {
			status = "PASS"
		}
	}
	header := bold.Render(status)
	if e.Status == "failed" || status == "FAIL" {
		header = m.accent("#A84F39", "#DB937C").Render(status)
	}
	if e.Training {
		header += " · ★ Training"
	}
	text := header + "\n" + safe(e.Definition.Name) + fmt.Sprintf(" · %s · revision %d", e.Definition.Kind, e.Definition.Revision) + "\nJudge: " + safe(e.Definition.Model) + "\n\n"
	if len(e.GenerationModels) > 0 {
		text += "Generated by: " + safe(strings.Join(e.GenerationModels, " · ")) + "\n\n"
	}
	text += "Criteria\n" + safe(e.Definition.Spec) + "\n\n"
	if e.Error != "" {
		text += m.accent("#A84F39", "#DB937C").Render(safe(e.Error)) + "\n\n"
	}
	if e.Result.Reason != "" {
		text += "Result\n" + safe(e.Result.Reason) + "\n\n"
	}
	if e.Result.Evidence != "" {
		text += "Evidence\n" + m.humanStyle().Render(safe(e.Result.Evidence)) + "\n\n"
	}
	if e.Note != "" {
		text += "Notes\n" + safe(e.Note) + "\n\n"
	}
	text += "Evaluated text\n\n" + safe(e.Text)
	return ansi.Wrap(text, width, "")
}
func (m *model) evaluationActions() tea.Cmd {
	if len(m.evaluationIDs()) == 0 {
		return nil
	}
	m.dialog = &dialog{kind: "eval-actions", title: fmt.Sprintf("%d selected", len(m.evaluationIDs())), rows: []row{
		{id: "keep", label: "Mark for training"}, {id: "remove", label: "Unmark for training"}, {id: "eval", label: "Evaluate again"}, {id: "notes", label: "Edit note (highlighted item)"}, {id: "inspect", label: "Exact inputs and results (highlighted item)"},
	}}
	return nil
}
func (m *model) evalAction(id string) tea.Cmd {
	ids := m.evaluationIDs()
	switch id {
	case "keep", "remove":
		return m.send("evaluation.annotate", map[string]any{"ids": ids, "training": id == "keep"})
	case "snapshot":
		return m.send("evaluation.export", nil)
	case "notes":
		if m.evaluation == nil || m.evaluation.ID != m.targetRow().id {
			return m.previewTarget()
		}
		m.evalEditingID = m.evaluation.ID
		m.editing = "evaluation-note"
		m.editor.SetValue(m.evaluation.Note)
		m.focus = 1
		m.reflow()
		return m.editor.Focus()
	case "inspect":
		if m.evaluation == nil || m.evaluation.ID != m.targetRow().id {
			return m.previewTarget()
		}
		var pretty bytes.Buffer
		json.Indent(&pretty, m.evaluationRaw, "", "  ")
		m.inspection = pretty.String()
		m.showInspector = true
		m.focus = 2
		m.reflow()
		return nil
	case "eval":
		return m.openEval("/eval")
	}
	return nil
}

// All forms reuse Carla's existing parent/back stack and correlated saves.
func (m *model) submitEvaluation(d *dialog) tea.Cmd {
	if len(d.fields) > 0 {
		if d.kind == "eval-new" {
			if strings.TrimSpace(d.fields[0].input.Value()) == "" || strings.TrimSpace(d.fields[1].input.Value()) == "" {
				m.status = "Enter a name and criteria"
				return nil
			}
			if d.args["kind"] == "llm" && len(m.data.SelectorModels) == 0 {
				m.status = "Configure a local policy model first"
				return nil
			}
			args := map[string]any{"name": d.fields[0].input.Value(), "spec": d.fields[1].input.Value(), "kind": d.args["kind"], "prompt": m.data.EvaluationPrompt, "threshold": 0.8, "model": ""}
			if args["kind"] == "llm" {
				args["model"] = m.data.SelectorModels[0].Alias
			}
			if args["kind"] == "jev" {
				args["model"] = m.simString("monitor_model")
			}
			return m.saveDialog(d, "evaluation.configure", args)
		}
		e := m.evaluator(d.args["id"].(string))
		args := e.args()
		field := d.args["field"].(string)
		value := d.fields[0].input.Value()
		if strings.TrimSpace(value) == "" {
			m.status = "Enter a value"
			return nil
		}
		args[field] = value
		return m.saveDialog(d, "evaluation.configure", args)
	}
	if len(d.rows) == 0 {
		return nil
	}
	r := d.rows[d.index]
	parent := func(cmd tea.Cmd) tea.Cmd {
		if m.dialog != nil {
			m.dialog.parent = d
		}
		return cmd
	}
	switch d.kind {
	case "policy":
		switch r.id {
		case "monitor":
			return parent(m.openLoomPolicy())
		case "selection":
			return parent(m.openSelectionConfig())
		default:
			return parent(m.openEvaluators())
		}
	case "eval-definitions":
		if r.id == "new" {
			m.dialog = &dialog{kind: "eval-new-kind", title: "Judge", parent: d, rows: []row{{id: "llm", label: "Local LLM", preview: "Prompt, criteria, boolean pass and quoted evidence."}, {id: "jev", label: "Jev via OpenRouter", preview: "Behavior spec and probability threshold. Uses OPENROUTER_API_KEY."}}}
			return nil
		}
		return parent(m.openEvaluator(r.id))
	case "eval-new-kind":
		n := &dialog{kind: "eval-new", title: "New evaluation", parent: d.parent, args: map[string]any{"kind": r.id}}
		n.add("Name", "")
		n.add("Criteria / behavior spec", "")
		m.dialog = n
		return n.fields[0].input.Focus()
	case "eval-definition":
		e := m.evaluator(d.args["id"].(string))
		switch r.id {
		case "threshold":
			m.numberConfig("evaluation", "threshold", e.Threshold*100, e.ID)
			m.dialog.title = "Pass probability · ↑↓ 1% · ←→ 10%"
			m.dialog.parent = d
			return nil
		case "spec", "prompt":
			m.evalEditingID = e.ID
			m.editing = "evaluation-" + r.id
			m.editReturn = d
			m.dialog = nil
			text := e.Spec
			if r.id == "prompt" {
				text = e.Prompt
			}
			m.editor.SetValue(text)
			m.focus = 1
			m.reflow()
			return m.editor.Focus()
		case "kind":
			m.dialog = &dialog{kind: "eval-kind", title: "Judge", parent: d, args: d.args, rows: []row{{id: "llm", label: "Local LLM"}, {id: "jev", label: "Jev via OpenRouter"}}}
			return nil
		case "model":
			if e.Kind == "llm" {
				n := &dialog{kind: "eval-model", title: "Local judge", parent: d, args: d.args}
				for _, model := range m.data.SelectorModels {
					n.rows = append(n.rows, row{id: model.Alias, label: model.Name})
				}
				m.dialog = n
				return nil
			}
		case "delete":
			m.dialog = &dialog{kind: "eval-delete", title: "Delete definition? Results are retained", parent: d, args: d.args, rows: []row{{id: "cancel", label: "Cancel"}, {id: "delete", label: "Delete definition"}}}
			return nil
		}
		n := &dialog{kind: "eval-field", title: r.label, parent: d, args: map[string]any{"id": e.ID, "field": r.id}}
		value := e.Name
		if r.id == "model" {
			value = e.Model
		}
		if r.id == "threshold" {
			value = fmt.Sprint(e.Threshold * 100)
		}
		n.add(r.label, value)
		m.dialog = n
		return n.fields[0].input.Focus()
	case "eval-kind", "eval-model":
		e := m.evaluator(d.args["id"].(string))
		args := e.args()
		if d.kind == "eval-model" {
			args["model"] = r.id
		} else {
			args["kind"] = r.id
			if r.id == "jev" {
				args["model"] = m.simString("monitor_model")
			} else if len(m.data.SelectorModels) > 0 {
				args["model"] = m.data.SelectorModels[0].Alias
			}
		}
		return m.saveDialog(d, "evaluation.configure", args)
	case "eval-delete":
		if r.id == "cancel" {
			m.dialog = d.parent
			return nil
		}
		m.dialog = d.parent.parent
		return m.send("evaluation.definition.delete", d.args)
	case "eval-run":
		args := d.args
		args["definition"] = r.id
		// Claim the request before navigation can request a preview. Otherwise
		// the preview's pending flag can silently drop a same-tab rerun.
		cmd := m.send("evaluation.run", args)
		if cmd != nil {
			m.dialog = nil
			m.switchSection(4)
			m.evalStarting = true
			m.evalFilter, m.filter = "", ""
			m.evalSelection = map[string]bool{}
		}
		return cmd
	case "eval-filter":
		m.evalFilter = r.id
		m.evalSelection = map[string]bool{}
		m.selected = 0
		m.dialog = nil
		m.reflow()
		return m.previewTarget()
	case "eval-actions":
		m.dialog = nil
		return m.evalAction(r.id)
	}
	return nil
}
